package proxysql

import (
	"fmt"
	"sort"
	"time"

	"github.com/github/freno/pkg/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/outbrain/golib/log"
	"github.com/patrickmn/go-cache"
)

const ignoreServerCacheCleanupTTL = time.Duration(500) * time.Millisecond

// MySQLServer represents a row in the main.runtime_mysql_servers table
type MySQLServer struct {
	Hostname string `db:"hostname"`
	Port     uint32 `db:"port"`
	Status   string `db:"status"`
}

// Address returns a string of the hostname/port of a server
func (ms *MySQLServer) Address() string {
	return fmt.Sprintf("%s:%d", ms.Hostname, ms.Port)
}

// Client is the ProxySQL Admin client
type Client struct {
	user                   string
	password               string
	dbs                    map[string]*sqlx.DB
	defaultIgnoreServerTTL time.Duration
	ignoreServerCache      *cache.Cache
}

// NewClient returns a ProxySQL Admin client
func NewClient(defaultIgnoreServerTTL time.Duration) *Client {
	return &Client{
		dbs:                    make(map[string]*sqlx.DB, 0),
		defaultIgnoreServerTTL: defaultIgnoreServerTTL,
		ignoreServerCache:      cache.New(cache.NoExpiration, ignoreServerCacheCleanupTTL),
	}
}

// GetDB returns a configured ProxySQL Admin connection
func (c *Client) GetDB(settings config.ProxySQLConfigurationSettings) (*sqlx.DB, string, error) {
	addrs := settings.Addresses
	sort.Strings(addrs)

	var err error
	for _, addr := range addrs {
		if db, found := c.dbs[addr]; found {
			return db, addr, nil
		}
		if db, err := sqlx.Connect("mysql", fmt.Sprintf("%s:%s@tcp(%s)/main", settings.User, settings.Password, addr)); err == nil {
			c.dbs[addr] = db
			return c.dbs[addr], addr, nil
		}

	}
	return nil, "", err
}

// CloseDB closes a ProxySQL Admin connection based on an address string
func (c *Client) CloseDB(addr string) {
	if db, found := c.dbs[addr]; found {
		db.Close()
		delete(c.dbs, addr)
	}
}

// GetReplicationHostgroupServers returns a list of MySQLServers for a replication hostgroup, based on the 'comment' field
func (c *Client) GetReplicationHostgroupServers(db *sqlx.DB, settings config.ProxySQLConfigurationSettings) (servers []*MySQLServer, err error) {
	allServers := make([]*MySQLServer, 0)
	if settings.HostgroupID == 0 {
		err = db.Select(&allServers, fmt.Sprintf(`SELECT ms.hostname, ms.port, ms.status
			FROM main.runtime_mysql_replication_hostgroups rhg
			JOIN main.runtime_mysql_servers ms ON rhg.reader_hostgroup=ms.hostgroup_id
			WHERE rhg.comment='%s'`, settings.HostgroupComment))
	} else {
		err = db.Select(&allServers, fmt.Sprintf(`SELECT ms.hostname, ms.port, ms.status
			FROM main.runtime_mysql_servers ms
			WHERE ms.hostgroup_id=%d`, settings.HostgroupID))
	}
	if err != nil {
		return servers, err
	}

	ignoreServerTTL := c.defaultIgnoreServerTTL
	if settings.IgnoreServerTTLSecs > 0 {
		ignoreServerTTL = time.Duration(settings.IgnoreServerTTLSecs) * time.Second
	}

	for _, server := range allServers {
		switch server.Status {
		case "ONLINE":
			if _, ignore := c.ignoreServerCache.Get(server.Address()); !ignore {
				servers = append(servers, server)
			} else {
				log.Debugf("found %q in the proxysql ignore-server cache, ignoring for %s", server.Address(), ignoreServerTTL)
			}
		default:
			c.ignoreServerCache.Set(server.Address(), true, ignoreServerTTL)
		}
	}

	return servers, nil
}
