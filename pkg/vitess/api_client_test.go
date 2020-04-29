package vitess

import (
	"encoding/json"
	"fmt"
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
		case "/api/keyspace/test/tablets/00":
			data, _ := json.Marshal([]Tablet{
				{
					MysqlHostname: "master",
					Type:          topodata.TabletType_MASTER,
				},
				{
					MysqlHostname: "replica",
					Type:          topodata.TabletType_REPLICA,
				},
				{
					MysqlHostname: "spare",
					Type:          topodata.TabletType_SPARE,
				},
				{
					MysqlHostname: "batch",
					Type:          topodata.TabletType_BATCH,
				},
				{
					MysqlHostname: "backup",
					Type:          topodata.TabletType_BACKUP,
				},
				{

					MysqlHostname: "restore",
					Type:          topodata.TabletType_RESTORE,
				},
			})
			fmt.Fprint(w, string(data))
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "[]")
		}
	}))
	defer vitessApi.Close()

	t.Run("success", func(t *testing.T) {
		tablets, err := ParseTablets(config.VitessConfigurationSettings{
			API:         vitessApi.URL,
			Keyspace:    "test",
			Shard:       "00",
			TimeoutSecs: 1.0,
		})
		if err != nil {
			t.Fatalf("Expected no error, got %q", err)
		}

		if len(tablets) != 1 {
			t.Fatalf("Expected 1 tablet, got %d", len(tablets))
		}

		if tablets[0].MysqlHostname != "replica" {
			t.Fatalf("Expected hostname %q, got %q", "replica", tablets[0].MysqlHostname)
		}

		if httpClient.Timeout != time.Second {
			t.Fatalf("Expected vitess client timeout of %v, got %v", time.Second, httpClient.Timeout)
		}
	})

	t.Run("not-found", func(t *testing.T) {
		tablets, err := ParseTablets(config.VitessConfigurationSettings{
			API:         vitessApi.URL,
			Keyspace:    "not-found",
			Shard:       "40-80",
			TimeoutSecs: 0.0,
		})
		if err != nil {
			t.Fatalf("Expected no error, got %q", err)
		}

		if len(tablets) > 0 {
			t.Fatalf("Expected 0 tablets, got %d", len(tablets))
		}

		if httpClient.Timeout != defaultTimeout {
			t.Fatalf("Expected vitess client timeout of %v, got %v", defaultTimeout, httpClient.Timeout)
		}
	})

	t.Run("failed", func(t *testing.T) {
		vitessApi.Close() // kill the mock vitess API
		_, err := ParseTablets(config.VitessConfigurationSettings{
			API:         vitessApi.URL,
			Keyspace:    "fail",
			Shard:       "00",
			TimeoutSecs: 0.0,
		})
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}
