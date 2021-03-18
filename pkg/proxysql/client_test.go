package proxysql

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/github/freno/pkg/config"
	"github.com/patrickmn/go-cache"
)

func TestProxySQLNewClient(t *testing.T) {
	c := NewClient(time.Second)
	if c.defaultIgnoreServerTTL != time.Second {
		t.Fatalf("expected 'defaultIgnoreServerTTL' to be 1s, got %v", c.defaultIgnoreServerTTL)
	}
	if c.ignoreServerCache == nil {
		t.Fatal("expected 'ignoreServerCache' to be created, got nil")
	}
}

func TestProxySQLGetDB(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDb, _, _ := sqlmock.New()
		c := &Client{
			dbs: map[string]*sql.DB{
				"127.0.0.1:3306": mockDb,
			},
		}
		db, addr, err := c.GetDB(config.ProxySQLConfigurationSettings{
			Addresses: []string{"127.0.0.1:3306"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if addr != "127.0.0.1:3306" {
			t.Fatalf("expected %q, got %q", "127.0.0.1:3306", addr)
		}
		if db == nil {
			t.Fatal("expected non-nil db")
		}
	})

	t.Run("failure", func(t *testing.T) {
		c := &Client{}
		_, _, err := c.GetDB(config.ProxySQLConfigurationSettings{
			Addresses: []string{"this.should.fail:3306"},
		})
		if err == nil {
			t.Fatal("expected error for failed connection")
		}
		if !strings.HasSuffix(err.Error(), "no such host") {
			t.Fatalf("expected a 'no such host' error, got %v", err)
		}
	})
}

func TestProxySQLCloseDB(t *testing.T) {
	db, _, _ := sqlmock.New()
	c := &Client{
		dbs: map[string]*sql.DB{
			"test": db,
		},
	}
	c.CloseDB("test")
	if len(c.dbs) != 0 {
		t.Fatalf("expected zero db conns, got %d", len(c.dbs))
	}
}

func TestProxySQLGetServers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"srv_host", "srv_port", "status"}).
			AddRow("replica1", 3306, "OFFLINE_SOFT").
			AddRow("replica2", 3306, "ONLINE").
			AddRow("replica3", 3306, "SHUNNED").
			AddRow("replica4", 3306, "ONLINE").
			AddRow("replica5", 3306, "SHUNNED_REPLICATION_LAG")
		mock.ExpectQuery(`SELECT srv_host, srv_port, status FROM stats_mysql_connection_pool WHERE hostgroup=123`).WillReturnRows(rows)

		c := &Client{
			ignoreServerCache: cache.New(cache.NoExpiration, time.Second),
		}

		servers, err := c.GetServers(db, config.ProxySQLConfigurationSettings{
			Addresses:   []string{"127.0.0.1:3306"},
			HostgroupID: 123,
		})
		if err != nil {
			t.Fatalf("got unexpected error: %v", err)
		}

		if len(servers) != 3 {
			t.Fatalf("expected 3 servers, got %d", len(servers))
		}
		for _, server := range servers {
			if server.Status != "ONLINE" && server.Status != "SHUNNED_REPLICATION_LAG" {
				t.Fatalf("expected servers to have status ONLINE or SHUNNED_REPLICATION_LAG, got %q", server.Status)
			}
		}
	})

	t.Run("ignored", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"srv_host", "srv_port", "status"}).
			AddRow("replica1", 3306, "SHUNNED_REPLICATION_LAG").
			AddRow("replica2", 3306, "ONLINE").
			AddRow("replica3", 3306, "ONLINE")
		mock.ExpectQuery(`SELECT srv_host, srv_port, status FROM stats_mysql_connection_pool WHERE hostgroup=321`).WillReturnRows(rows)

		c := &Client{
			ignoreServerCache:      cache.New(cache.NoExpiration, time.Second),
			defaultIgnoreServerTTL: time.Second,
		}
		c.ignoreServerCache.Set("replica3:3306", true, cache.NoExpiration) // this host should be ignored

		servers, err := c.GetServers(db, config.ProxySQLConfigurationSettings{
			Addresses:   []string{"127.0.0.1:3306"},
			HostgroupID: 321,
		})
		if err != nil {
			t.Fatalf("got unexpected error: %v", err)
		}

		if len(servers) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(servers))
		}

		for _, replica := range servers {
			switch replica.Host {
			case "replica1":
				if replica.Status != "SHUNNED_REPLICATION_LAG" {
					t.Fatalf("expected server to have status %q, got %q", "SHUNNED_REPLICATION_LAG", replica.Status)
				}
			case "replica2":
				if replica.Status != "ONLINE" {
					t.Fatalf("expected server to have status %q, got %q", "ONLINE", replica.Status)
				}
			default:
				t.Fatalf("got unexpected replica %v", replica)
			}
		}
	})
}
