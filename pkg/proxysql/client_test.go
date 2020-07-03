package proxysql

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/github/freno/pkg/config"
	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
)

func TestGetReplicationHostgroupServers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
