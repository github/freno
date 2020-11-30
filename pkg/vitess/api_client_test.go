package vitess

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/github/freno/pkg/config"
	"vitess.io/vitess/go/vt/proto/topodata"
)

func TestParseTablets(t *testing.T) {
	vitessApi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.String() {
		case "/api/keyspace/test/tablets/00", "/api/keyspace/test/tablets/00?cells=cell2":
			json.NewEncoder(w).Encode([]Tablet{
				{
					// primary
					Alias: &topodata.TabletAlias{Cell: "cell1"},
					Type:  topodata.TabletType_MASTER,
				},
				{
					// replica without tablet stats
					Alias:         &topodata.TabletAlias{Cell: "cell2"},
					MysqlHostname: "replica1",
					Type:          topodata.TabletType_REPLICA,
				},
				{
					// replica with healthy tablet stats
					Alias:         &topodata.TabletAlias{Cell: "cell3"},
					MysqlHostname: "replica2",
					Stats: &TabletStats{
						Realtime: &TabletRealtimeStats{},
						Serving:  true,
						Up:       true,
					},
					Type: topodata.TabletType_REPLICA,
				},
				{
					// replica with nil realtime stats (ignore)
					Alias:         &topodata.TabletAlias{Cell: "cell1"},
					MysqlHostname: "replica3",
					Stats: &TabletStats{
						Realtime: nil,
						Serving:  true,
						Up:       true,
					},
				},
				{
					// replica with tablet stats + not serving (replication not running)
					Alias:         &topodata.TabletAlias{Cell: "cell2"},
					MysqlHostname: "replica4",
					Stats: &TabletStats{
						LastError: "vttablet error: replication is not running",
						Realtime: &TabletRealtimeStats{
							HealthError: "replication is not running",
						},
						Serving: false,
						Up:      true,
					},
					Type: topodata.TabletType_REPLICA,
				},
				{
					// spare tablet
					Alias: &topodata.TabletAlias{Cell: "cell2"},
					Type:  topodata.TabletType_SPARE,
				},
				{
					// batch tablet
					Alias: &topodata.TabletAlias{Cell: "cell3"},
					Type:  topodata.TabletType_BATCH,
				},
				{
					// backup tablet
					Alias: &topodata.TabletAlias{Cell: "cell2"},
					Type:  topodata.TabletType_BACKUP,
				},
				{
					// restore tablet
					Alias: &topodata.TabletAlias{Cell: "cell1"},
					Type:  topodata.TabletType_RESTORE,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer vitessApi.Close()

	t.Run("success", func(t *testing.T) {
		tablets, err := ParseTablets(config.VitessConfigurationSettings{
			API:         vitessApi.URL,
			Keyspace:    "test",
			Shard:       "00",
			TimeoutSecs: 1,
		})
		if err != nil {
			t.Fatalf("Expected no error, got %q", err)
		}

		if len(tablets) != 2 {
			t.Fatalf("Expected 2 tablets, got %d", len(tablets))
		}

		if tablets[0].MysqlHostname != "replica1" {
			t.Fatalf("Expected hostname %q, got %q", "replica1", tablets[0].MysqlHostname)
		}
		if tablets[1].MysqlHostname != "replica2" {
			t.Fatalf("Expected hostname %q, got %q", "replica2", tablets[1].MysqlHostname)
		}

		if httpClient.Timeout != time.Second {
			t.Fatalf("Expected vitess client timeout of %v, got %v", time.Second, httpClient.Timeout)
		}
	})

	t.Run("with-cell", func(t *testing.T) {
		settings := config.VitessConfigurationSettings{
			API:      vitessApi.URL,
			Cells:    []string{"cell2"},
			Keyspace: "test",
			Shard:    "00",
		}
		tablets, err := ParseTablets(settings)
		if err != nil {
			t.Fatalf("Expected no error, got %q", err)
		}

		if len(tablets) != 1 {
			t.Fatalf("Expected 1 tablet, got %d", len(tablets))
		}

		if tablets[0].MysqlHostname != "replica1" {
			t.Fatalf("Expected hostname %q, got %q", "replica1", tablets[0].MysqlHostname)
		}
		if tablets[0].Alias.GetCell() != "cell2" {
			t.Fatalf("Expected vitess cell %s, got %s", "cell2", tablets[0].Alias.GetCell())
		}

		// empty cell names should cause no filtering
		settings.Cells = []string{"", ""}
		tablets, _ = ParseTablets(settings)
		if len(tablets) != 2 {
			t.Fatalf("Expected 2 tablet, got %d", len(tablets))
		}
	})

	t.Run("not-found", func(t *testing.T) {
		tablets, err := ParseTablets(config.VitessConfigurationSettings{
			API:      vitessApi.URL,
			Keyspace: "not-found",
			Shard:    "40-80",
		})
		if err == nil || err.Error() != "404 Not Found" {
			t.Fatalf("Expected %q error, got %q", "404 Not Found", err)
		}

		if len(tablets) != 0 {
			t.Fatalf("Expected 0 tablets, got %d", len(tablets))
		}

		if httpClient.Timeout != defaultTimeout {
			t.Fatalf("Expected vitess client timeout of %v, got %v", defaultTimeout, httpClient.Timeout)
		}
	})

	t.Run("failed", func(t *testing.T) {
		vitessApi.Close() // kill the mock vitess API
		_, err := ParseTablets(config.VitessConfigurationSettings{
			API:      vitessApi.URL,
			Keyspace: "fail",
			Shard:    "00",
		})
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}
