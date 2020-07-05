package proxysql

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/github/freno/pkg/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/outbrain/golib/log"
	"github.com/patrickmn/go-cache"
)

const ignoreServerCacheCleanupTTL = time.Duration(500) * time.Millisecond

// MySQLConnectionPoolServer represents a row in the stats_mysql_connection_pool table
type MySQLConnectionPoolServer struct {
	Host   string
	Port   int32
	Status string
}

// Address returns a string of the hostname/port of a server
func (ms *MySQLConnectionPoolServer) Address() string {
	return fmt.Sprintf("%s:%d", ms.Host, ms.Port)
}

// Client is the ProxySQL Admin client
type Client struct {
	dbs                    map[string]*sql.DB
	defaultIgnoreServerTTL time.Duration
	ignoreServerCache      *cache.Cache
}

// NewClient returns a ProxySQL Admin client
func NewClient(defaultIgnoreServerTTL time.Duration) *Client {
	return &Client{
		dbs:                    make(map[string]*sql.DB, 0),
		defaultIgnoreServerTTL: defaultIgnoreServerTTL,
		ignoreServerCache:      cache.New(cache.NoExpiration, ignoreServerCacheCleanupTTL),
	}
}

// GetDB returns a configured ProxySQL Admin connection
func (c *Client) GetDB(settings config.ProxySQLConfigurationSettings) (*sql.DB, string, error) {
	addrs := settings.Addresses
	sort.Strings(addrs)

	var lastErr error
	for _, addr := range addrs {
		if db, found := c.dbs[addr]; found {
			return db, addr, nil
		}
		db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/stats?interpolateParams=true&timeout=500ms",
			settings.User, settings.Password, addr,
		))
		if err != nil {
			lastErr = err
			continue
		}
		if err = db.Ping(); err != nil {
			lastErr = err
			continue
		}
		log.Debugf("connected to ProxySQL at mysql://%s/stats", addr)
		c.dbs[addr] = db
		return c.dbs[addr], addr, nil
	}
	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", errors.New("failed to get connection")
}

// CloseDB closes a ProxySQL Admin connection based on an address string
func (c *Client) CloseDB(addr string) {
	if db, found := c.dbs[addr]; found {
		db.Close()
		delete(c.dbs, addr)
	}
}

// GetConnectionPoolServers returns a list of MySQLConnectionPoolServers based on a hostgroup ID
func (c *Client) GetConnectionPoolServers(db *sql.DB, settings config.ProxySQLConfigurationSettings) (servers []*MySQLConnectionPoolServer, err error) {
	ignoreServerTTL := c.defaultIgnoreServerTTL
	if settings.IgnoreServerTTLSecs > 0 {
		ignoreServerTTL = time.Duration(settings.IgnoreServerTTLSecs) * time.Second
	}

	rows, err := db.Query(fmt.Sprintf(`SELECT srv_host, srv_port, status FROM stats_mysql_connection_pool WHERE hostgroup=%d`, settings.HostgroupID))
	if err != nil {
		return servers, err
	}
	defer rows.Close()
	allServers := make([]*MySQLConnectionPoolServer, 0)
	for rows.Next() {
		server := &MySQLConnectionPoolServer{}
		if err = rows.Scan(&server.Host, &server.Port, &server.Status); err != nil {
			return nil, err
		}
		allServers = append(allServers, server)
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
