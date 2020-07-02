package proxysql

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/github/freno/pkg/config"
	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
)

func TestGetRHGServers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		rows := sqlmock.NewRows([]string{"hostname", "port", "status", "table_version"}).
			AddRow("replica1", 3306, "ONLINE", 1).
			AddRow("replica2", 3306, "SHUNNED", 1)
		mock.ExpectQuery(`SELECT ms.hostname, ms.port, ms.status, smg.variable_value AS table_version
	        FROM main.runtime_mysql_replication_hostgroups rhg
	        JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
	        JOIN stats.stats_mysql_global smg ON smg.variable_name='Servers_table_version'
		WHERE rhg.comment='success'`).WillReturnRows(rows)

		c := &Client{
			dbs: map[string]*sqlx.DB{
				"127.0.0.1:3306": sqlx.NewDb(db, ""),
			},
			ignoreServerCache: cache.New(cache.NoExpiration, time.Second),
		}

		servers, err := c.GetRHGServers(config.ProxySQLConfigurationSettings{
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
		rows := sqlmock.NewRows([]string{"hostname", "port", "status", "table_version"}).
			AddRow("replica1", 3306, "ONLINE", 1).
			AddRow("replica2", 3306, "ONLINE", 1)
		mock.ExpectQuery(`SELECT ms.hostname, ms.port, ms.status, smg.variable_value AS table_version
	        FROM main.runtime_mysql_replication_hostgroups rhg
	        JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
	        JOIN stats.stats_mysql_global smg ON smg.variable_name='Servers_table_version'
		WHERE rhg.comment='ignored'`).WillReturnRows(rows)

		c := &Client{
			dbs:               map[string]*sqlx.DB{"127.0.0.1:3306": sqlx.NewDb(db, "")},
			ignoreServerCache: cache.New(cache.NoExpiration, time.Second),
		}
		c.ignoreServerCache.Set("replica1:3306", true, cache.DefaultExpiration) // this host should be ignored

		servers, err := c.GetRHGServers(config.ProxySQLConfigurationSettings{
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
