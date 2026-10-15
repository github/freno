package proxysql

import (
	"database/sql"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/github/freno/pkg/config"
)

func capacityTestSettings() config.ProxySQLCapacityConfigurationSettings {
	return config.ProxySQLCapacityConfigurationSettings{
		Metric: "frontend", FrontendMaxConnections: 200000, WriterHostgroupID: 30,
		BackendLimits: []config.ProxySQLBackendLimit{
			{Host: "/var/run/mysqld/mysqld.sock", Port: 0, MaxConnections: 1000},
		},
	}
}

func capacityTestSampler(t *testing.T) (*CapacitySampler, sqlmock.Sqlmock, *time.Time) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	now := time.Now()
	sampler := NewCapacitySampler()
	sampler.now = func() time.Time { return now }
	sampler.openDB = func(address, user, password string) (*sql.DB, error) {
		if address != "proxy.example:6032" || user != "stats" || password != "test-only" {
			t.Errorf("unexpected capacity endpoint or credentials")
		}
		return db, nil
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.ExpectClose()
		if err := sampler.Close(); err != nil {
			t.Error(err)
		}
	})
	return sampler, mock, &now
}

func expectCapacitySnapshot(mock sqlmock.Sqlmock, connected interface{}, backends *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(frontendCapacityQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"Variable_Value"}).AddRow(connected))
	mock.ExpectQuery(regexp.QuoteMeta(backendCapacityQuery)).WithArgs(30).WillReturnRows(backends)
}

func writerRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"srv_host", "srv_port", "status", "ConnUsed"})
}

func TestCapacitySharedSnapshotAndPollingBound(t *testing.T) {
	sampler, mock, now := capacityTestSampler(t)
	expectCapacitySnapshot(mock, "150000", writerRows().AddRow("/var/run/mysqld/mysqld.sock", 0, "ONLINE", 900))
	settings := capacityTestSettings()
	for i := 0; i < 100; i++ {
		value, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings)
		if err != nil || value != 75 {
			t.Fatalf("frontend = %v, %v; want 75", value, err)
		}
		settings.Metric = "writer-backend"
		value, err = sampler.Read("proxy.example", 6032, "stats", "test-only", settings)
		if err != nil || value != 90 {
			t.Fatalf("writer = %v, %v; want 90", value, err)
		}
		settings.Metric = "frontend"
	}
	*now = now.Add(CapacityPollInterval - time.Nanosecond)
	if _, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Nanosecond)
	expectCapacitySnapshot(mock, 100000, writerRows().AddRow("/var/run/mysqld/mysqld.sock", 0, "ONLINE", 500))
	value, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings)
	if err != nil || value != 50 {
		t.Fatalf("new sample = %v, %v; want 50", value, err)
	}
}

func TestCapacityFailureIsCachedAndReplacesHealthySnapshot(t *testing.T) {
	sampler, mock, now := capacityTestSampler(t)
	settings := capacityTestSettings()
	expectCapacitySnapshot(mock, 100000, writerRows().AddRow("/var/run/mysqld/mysqld.sock", 0, "ONLINE", 500))
	if _, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(CapacityPollInterval)
	mock.ExpectQuery(regexp.QuoteMeta(frontendCapacityQuery)).WillReturnError(errors.New("stats unavailable"))
	for i := 0; i < 100; i++ {
		for _, metric := range []string{"frontend", "writer-backend"} {
			settings.Metric = metric
			if _, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings); err == nil {
				t.Fatal("failed poll must not return a healthy cached sample")
			}
		}
	}
	*now = now.Add(CapacityPollInterval)
	expectCapacitySnapshot(mock, 80000, writerRows().AddRow("/var/run/mysqld/mysqld.sock", 0, "ONLINE", 400))
	if value, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings); err != nil || value != 40 {
		t.Fatalf("recovery sample = %v, %v", value, err)
	}
}

func TestCapacityWriterCoverageAndWorstBackend(t *testing.T) {
	tests := []struct {
		name    string
		rows    *sqlmock.Rows
		want    float64
		wantErr bool
	}{
		{"worst, not average", writerRows().AddRow("db1", 3306, "ONLINE", 100).AddRow("db2", 3306, "ONLINE", 950), 95, false},
		{"missing writer", writerRows().AddRow("db1", 3306, "ONLINE", 100), 0, true},
		{"empty pool", writerRows(), 0, true},
		{"shunned writer", writerRows().AddRow("db1", 3306, "ONLINE", 100).AddRow("db2", 3306, "SHUNNED", 0), 0, true},
		{"unknown online writer", writerRows().AddRow("db3", 3306, "ONLINE", 100), 0, true},
		{"duplicate writer", writerRows().AddRow("db1", 3306, "ONLINE", 1).AddRow("db1", 3306, "ONLINE", 2), 0, true},
		{"negative count", writerRows().AddRow("db1", 3306, "ONLINE", -1), 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sampler, mock, _ := capacityTestSampler(t)
			settings := capacityTestSettings()
			settings.Metric = "writer-backend"
			settings.BackendLimits = []config.ProxySQLBackendLimit{
				{Host: "db1", Port: 3306, MaxConnections: 1000},
				{Host: "db2", Port: 3306, MaxConnections: 1000},
			}
			expectCapacitySnapshot(mock, 100000, test.rows)
			value, err := sampler.Read("proxy.example", 6032, "stats", "test-only", settings)
			if (err != nil) != test.wantErr || value != test.want {
				t.Fatalf("got %v, %v; want %v, error=%v", value, err, test.want, test.wantErr)
			}
		})
	}
}

func TestCapacityInvalidFrontendStats(t *testing.T) {
	for _, counter := range []interface{}{nil, "not-a-number", int64(-1)} {
		t.Run("invalid counter", func(t *testing.T) {
			sampler, mock, _ := capacityTestSampler(t)
			mock.ExpectQuery(regexp.QuoteMeta(frontendCapacityQuery)).
				WillReturnRows(sqlmock.NewRows([]string{"Variable_Value"}).AddRow(counter))
			if _, err := sampler.Read("proxy.example", 6032, "stats", "test-only", capacityTestSettings()); err == nil {
				t.Fatal("invalid frontend stats must fail")
			}
		})
	}
}

func TestCapacityConcurrentReadsShareOnePoll(t *testing.T) {
	sampler, mock, _ := capacityTestSampler(t)
	expectCapacitySnapshot(mock, 100000, writerRows().AddRow("/var/run/mysqld/mysqld.sock", 0, "ONLINE", 500))
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := sampler.Read("proxy.example", 6032, "stats", "test-only", capacityTestSettings()); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
}

func TestCapacityQueryDeadline(t *testing.T) {
	sampler, mock, _ := capacityTestSampler(t)
	mock.ExpectQuery(regexp.QuoteMeta(frontendCapacityQuery)).
		WillDelayFor(2 * capacityQueryTimeout).
		WillReturnRows(sqlmock.NewRows([]string{"Variable_Value"}).AddRow(100))
	start := time.Now()
	if _, err := sampler.Read("proxy.example", 6032, "stats", "test-only", capacityTestSettings()); err == nil {
		t.Fatal("query deadline must fail the probe")
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("query deadline took %v, want approximately one second", elapsed)
	}
}

func TestCapacityRejectsApplicationEndpointsBeforeConnecting(t *testing.T) {
	sampler := NewCapacitySampler()
	sampler.openDB = func(_, _, _ string) (*sql.DB, error) {
		t.Fatal("must not connect to an application endpoint")
		return nil, nil
	}
	for _, port := range []int{3306, 6033} {
		if _, err := sampler.Read("primary.example", port, "stats", "test-only", capacityTestSettings()); err == nil {
			t.Fatal("application port must be rejected")
		}
	}
}
