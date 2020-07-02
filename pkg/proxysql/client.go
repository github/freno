package proxysql

import (
	"fmt"
	"sync"
	"time"

	"github.com/github/freno/pkg/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/outbrain/golib/log"
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
	user              string
	password          string
	dbs               map[string]*sqlx.DB
	ignoreServerCache *cache.Cache
	ignoreServerTTL   time.Duration
}

func NewClient(ignoreServerTTL time.Duration) *Client {
	return &Client{
		dbs:               make(map[string]*sqlx.DB, 0),
		ignoreServerCache: cache.New(ignoreServerTTL, time.Second),
		ignoreServerTTL:   ignoreServerTTL,
	}
}

func (c *Client) getDB(settings config.ProxySQLConfigurationSettings) (*sqlx.DB, error) {
	key := settings.URL()
	if db, found := c.dbs[key]; found {
		return db, nil
	}

	var err error
	for _, addr := range settings.Addresses {
		db, err := sqlx.Connect("mysql", fmt.Sprintf("%s:%s@tcp(%s)/main", settings.User, settings.Password, addr))
		if err == nil {
			c.dbs[key] = db
			return c.dbs[key], nil
		}

	}
	return nil, err
}

func (c *Client) GetRHGServers(settings config.ProxySQLConfigurationSettings) (servers []*MySQLServer, err error) {
	c.Lock()
	defer c.Unlock()

	db, err := c.getDB(settings)
	if err != nil {
		return servers, err
	}

	allServers := make([]*MySQLServer, 0)
	err = db.Select(&allServers, fmt.Sprintf(`SELECT ms.hostname, ms.port, ms.status, smg.variable_value AS table_version
		FROM main.runtime_mysql_replication_hostgroups rhg
		JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
		JOIN stats.stats_mysql_global smg ON smg.variable_name='Servers_table_version'
		WHERE rhg.comment='%s'`, settings.HostgroupComment))
	if err != nil {
		return servers, err
	}

	for _, server := range allServers {
		switch server.Status {
		case MySQLServerOnline:
			if _, ignore := c.ignoreServerCache.Get(server.Addr()); !ignore {
				servers = append(servers, server)
			} else {
				log.Debugf("host %q is in the proxysql ignore-server cache, ignoring for %s", server.Addr(), c.ignoreServerTTL)
			}
		default:
			c.ignoreServerCache.Set(server.Addr(), true, cache.DefaultExpiration)
		}
	}

	return servers, err
}
