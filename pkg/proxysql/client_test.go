package proxysql

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/github/freno/pkg/config"
	"github.com/jmoiron/sqlx"
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
			dbs: map[string]*sqlx.DB{
				"127.0.0.1:3306": sqlx.NewDb(mockDb, ""),
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
		if err.Error() != "dial tcp: lookup this.should.fail: no such host" {
			t.Fatalf("expected a 'no such host' error, got %v", err)
		}
	})
}

func TestProxySQLCloseDB(t *testing.T) {
	db, _, _ := sqlmock.New()
	c := &Client{
		dbs: map[string]*sqlx.DB{
			"test": sqlx.NewDb(db, ""),
		},
	}
	c.CloseDB("test")
	if len(c.dbs) != 0 {
		t.Fatalf("expected zero db conns, got %d", len(c.dbs))
	}
}

func TestProxySQLGetReplicationHostgroupServers(t *testing.T) {
	t.Run("hostgroup-comment", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"hostname", "port", "status"}).
			AddRow("replica1", 3306, "ONLINE").
			AddRow("replica2", 3306, "SHUNNED")
		mock.ExpectQuery(`SELECT ms.hostname, ms.port, ms.status
		        FROM main.runtime_mysql_replication_hostgroups rhg
		        JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
			WHERE rhg.comment='success'`).WillReturnRows(rows)

		c := &Client{
			ignoreServerCache: cache.New(cache.NoExpiration, time.Second),
		}

		servers, err := c.GetReplicationHostgroupServers(sqlx.NewDb(db, ""), config.ProxySQLConfigurationSettings{
			Addresses:        []string{"127.0.0.1:3306"},
			HostgroupComment: "success",
		})
		if err != nil {
			t.Fatalf("got unexpected error: %v", err)
		}

		if len(servers) != 1 {
			t.Fatalf("expected only 1 server, got %d", len(servers))
		}
		replica := servers[0]
		if replica.Hostname != "replica1" {
			t.Fatalf("expected host to have hostname %q, got %q", replica.Hostname, "replica1")
		}
	})

	t.Run("hostgroup-id", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"hostname", "port", "status"}).
			AddRow("replica1", 3306, "ONLINE").
			AddRow("replica2", 3306, "ONLINE")
		mock.ExpectQuery(`SELECT ms.hostname, ms.port, ms.status
		        FROM main.runtime_mysql_servers ms
			WHERE ms.hostgroup_id=1`).WillReturnRows(rows)

		c := &Client{
			ignoreServerCache: cache.New(cache.NoExpiration, time.Second),
		}

		servers, err := c.GetReplicationHostgroupServers(sqlx.NewDb(db, ""), config.ProxySQLConfigurationSettings{
			Addresses:   []string{"127.0.0.1:3306"},
			HostgroupID: 1,
		})
		if err != nil {
			t.Fatalf("got unexpected error: %v", err)
		}

		if len(servers) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(servers))
		}
	})

	t.Run("ignored", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"hostname", "port", "status"}).
			AddRow("replica1", 3306, "ONLINE").
			AddRow("replica2", 3306, "ONLINE")
		mock.ExpectQuery(`SELECT ms.hostname, ms.port, ms.status
		        FROM main.runtime_mysql_replication_hostgroups rhg
		        JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
			WHERE rhg.comment='ignored'`).WillReturnRows(rows)

		c := &Client{
			ignoreServerCache:      cache.New(cache.NoExpiration, time.Second),
			defaultIgnoreServerTTL: time.Second,
		}
		c.ignoreServerCache.Set("replica1:3306", true, cache.NoExpiration) // this host should be ignored

		servers, err := c.GetReplicationHostgroupServers(sqlx.NewDb(db, ""), config.ProxySQLConfigurationSettings{
			Addresses:        []string{"127.0.0.1:3306"},
			HostgroupComment: "ignored",
		})
		if err != nil {
			t.Fatalf("got unexpected error: %v", err)
		}

		if len(servers) != 1 {
			t.Fatalf("expected 1 server, got %d", len(servers))
		}

		replica := servers[0]
		if replica.Hostname != "replica2" {
			t.Fatalf("expected host to have hostname %q, got %q", replica.Hostname, "replica2")
		}
	})
}
