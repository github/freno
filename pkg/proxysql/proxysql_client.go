package proxysql

import (
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
)

type MySQLServerStatus string

const (
	MySQLServerOfflineHard MySQLServerStatus = "OFFLINE_HARD"
	MySQLServerOfflineSoft MySQLServerStatus = "OFFLINE_SOFT"
	MySQLServerOnline      MySQLServerStatus = "ONLINE"
	MySQLServerShunned     MySQLServerStatus = "SHUNNED"
)

type MySQLServer struct {
	Hostname     string            `db:"hostname"`
	Port         uint32            `db:"port"`
	Status       MySQLServerStatus `db:"status"`
	TableVersion uint64            `db:"table_version"`
}

func (ms *MySQLServer) Addr() string {
	return fmt.Sprintf("%s:%d", ms.Hostname, ms.Port)
}

type Client struct {
	sync.Mutex
	addrs             []string
	currentAddr       string
	user              string
	password          string
	db                *sqlx.DB
	ignoreServerCache *cache.Cache
}

func New(addrs []string, user, password string, ignoreServerTTL time.Duration) (*Client, error) {
	c := &Client{
		addrs:             addrs,
		user:              user,
		password:          password,
		ignoreServerCache: cache.New(ignoreServerTTL, time.Second),
	}
	return c, c.Reconnect()
}

func (c *Client) Close() {
	c.Lock()
	defer c.Unlock()

	if c.db != nil {
		c.db.Close()
	}
}

func (c *Client) Reconnect() (err error) {
	c.Lock()
	defer c.Unlock()

	for _, addr := range c.addrs {
		if addr == c.currentAddr {
			continue
		}
		db, err := sqlx.Connect("mysql", fmt.Sprintf("%s:%s@tcp(%s)/main", c.user, c.password, addr))
		if err == nil {
			c.currentAddr = addr
			c.db = db
			return nil
		}

	}
	return err
}

func (c *Client) GetRHGServers(rhgComment string) (servers []*MySQLServer, err error) {
	c.Lock()
	defer c.Unlock()

	allServers := make([]*MySQLServer, 0)
	err = c.db.Select(&allServers, fmt.Sprintf(`SELECT ms.hostname, ms.port, ms.status, smg.variable_value AS table_version
		FROM main.runtime_mysql_replication_hostgroups rhg
		JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
		JOIN stats.stats_mysql_global smg ON smg.variable_name='Servers_table_version'
		WHERE rhg.comment='%s'`, rhgComment))
	if err != nil {
		return servers, err
	}

	for _, server := range allServers {
		switch server.Status {
		case MySQLServerOnline:
			if _, ignore := c.ignoreServerCache.Get(server.Addr()); !ignore {
				servers = append(servers, server)
			}
		default:
			c.ignoreServerCache.Set(server.Addr(), 1, cache.DefaultExpiration)
		}
	}

	return servers, err
}
