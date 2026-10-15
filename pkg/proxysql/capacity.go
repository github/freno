package proxysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/github/freno/pkg/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/outbrain/golib/log"
)

const CapacityPollInterval = 5 * time.Second
const capacityQueryTimeout = time.Second
const capacityIdleTimeout = 10 * time.Minute

const frontendCapacityQuery = `SELECT Variable_Value FROM stats_mysql_global WHERE Variable_Name = 'Client_Connections_connected'`
const backendCapacityQuery = `SELECT srv_host, srv_port, status, ConnUsed FROM stats_mysql_connection_pool WHERE hostgroup = ?`

type capacityBackend struct {
	host   string
	port   int
	status string
	used   int64
}

type capacitySnapshot struct {
	frontend int64
	backends []capacityBackend
}

type capacityEndpoint struct {
	mu       sync.Mutex
	db       *sql.DB
	lastPoll time.Time
	lastUsed time.Time // guarded by CapacitySampler.mu
	snapshot capacitySnapshot
	err      error
}

// CapacitySampler shares one connection and one success/error snapshot between
// the frontend and backend probes for each proxy, credential set, and hostgroup.
type CapacitySampler struct {
	mu          sync.Mutex
	endpoints   map[string]*capacityEndpoint
	lastCleanup time.Time
	now         func() time.Time
	openDB      func(string, string, string) (*sql.DB, error)
}

func NewCapacitySampler() *CapacitySampler {
	return &CapacitySampler{
		endpoints: make(map[string]*capacityEndpoint),
		now:       time.Now,
		openDB:    openCapacityDB,
	}
}

func openCapacityDB(address, user, password string) (*sql.DB, error) {
	settings := mysqldriver.NewConfig()
	settings.Net = "tcp"
	settings.Addr = address
	settings.User = user
	settings.Passwd = password
	settings.DBName = config.ProxySQLDefaultDatabase
	settings.Timeout = capacityQueryTimeout
	settings.ReadTimeout = capacityQueryTimeout
	settings.WriteTimeout = capacityQueryTimeout
	settings.InterpolateParams = true
	db, err := sql.Open("mysql", settings.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(capacityIdleTimeout)
	return db, nil
}

func (sampler *CapacitySampler) endpoint(address, user, password string, hostgroup int) *capacityEndpoint {
	key := fmt.Sprintf("%s:%d:%x", address, hostgroup, sha256.Sum256([]byte(user+"\x00"+password)))
	now := sampler.now()
	sampler.mu.Lock()
	defer sampler.mu.Unlock()
	if now.Sub(sampler.lastCleanup) >= capacityIdleTimeout {
		for key, endpoint := range sampler.endpoints {
			if now.Sub(endpoint.lastUsed) >= capacityIdleTimeout {
				endpoint.mu.Lock()
				if endpoint.db != nil {
					if err := endpoint.db.Close(); err != nil {
						log.Errorf("closing idle ProxySQL capacity pool: %v", err)
					}
				}
				endpoint.mu.Unlock()
				delete(sampler.endpoints, key)
			}
		}
		sampler.lastCleanup = now
	}
	endpoint := sampler.endpoints[key]
	if endpoint == nil {
		endpoint = &capacityEndpoint{}
		sampler.endpoints[key] = endpoint
	}
	endpoint.lastUsed = now
	return endpoint
}

func (sampler *CapacitySampler) Close() error {
	sampler.mu.Lock()
	defer sampler.mu.Unlock()
	var firstErr error
	for key, endpoint := range sampler.endpoints {
		endpoint.mu.Lock()
		if endpoint.db != nil {
			if err := endpoint.db.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		endpoint.mu.Unlock()
		delete(sampler.endpoints, key)
	}
	return firstErr
}

func (sampler *CapacitySampler) Read(host string, port int, user, password string, settings config.ProxySQLCapacityConfigurationSettings) (float64, error) {
	if err := settings.Validate(); err != nil {
		return 0, err
	}
	if host == "" || port < 1 || port > 65535 || port == 3306 || port == 6033 || user == "" || password == "" {
		return 0, fmt.Errorf("ProxySQL capacity requires an explicit admin endpoint and stats credentials")
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	endpoint := sampler.endpoint(address, user, password, settings.WriterHostgroupID)
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	now := sampler.now()
	if endpoint.lastPoll.IsZero() || now.Sub(endpoint.lastPoll) >= CapacityPollInterval {
		endpoint.lastPoll = now
		endpoint.snapshot = capacitySnapshot{}
		endpoint.err = nil
		if endpoint.db == nil {
			endpoint.db, endpoint.err = sampler.openDB(address, user, password)
		}
		if endpoint.err == nil {
			endpoint.snapshot, endpoint.err = readCapacitySnapshot(endpoint.db, settings.WriterHostgroupID)
		}
	}
	if endpoint.err != nil {
		return 0, fmt.Errorf("ProxySQL stats at %s: %w", address, endpoint.err)
	}
	if settings.Metric == "frontend" {
		return 100 * float64(endpoint.snapshot.frontend) / float64(settings.FrontendMaxConnections), nil
	}
	limits := make(map[string]config.ProxySQLBackendLimit)
	for _, backend := range settings.BackendLimits {
		limits[net.JoinHostPort(backend.Host, strconv.Itoa(backend.Port))] = backend
	}
	seen := make(map[string]bool)
	worst := float64(0)
	for _, backend := range endpoint.snapshot.backends {
		key := net.JoinHostPort(backend.host, strconv.Itoa(backend.port))
		limit, expected := limits[key]
		if !expected && backend.status != "ONLINE" {
			continue
		}
		if !expected || seen[key] || backend.status != "ONLINE" {
			return 0, fmt.Errorf("writer backend %s is unknown, duplicated, or not ONLINE", key)
		}
		seen[key] = true
		value := 100 * float64(backend.used) / float64(limit.MaxConnections)
		if value > worst {
			worst = value
		}
	}
	if len(seen) != len(limits) {
		return 0, fmt.Errorf("ProxySQL writer stats do not contain every configured backend")
	}
	return worst, nil
}

func readCapacitySnapshot(db *sql.DB, hostgroup int) (capacitySnapshot, error) {
	snapshot := capacitySnapshot{}
	ctx, cancel := context.WithTimeout(context.Background(), capacityQueryTimeout)
	defer cancel()
	if err := db.QueryRowContext(ctx, frontendCapacityQuery).Scan(&snapshot.frontend); err != nil {
		return snapshot, fmt.Errorf("frontend connections: %w", err)
	}
	if snapshot.frontend < 0 {
		return snapshot, fmt.Errorf("frontend connections must not be negative")
	}
	rows, err := db.QueryContext(ctx, backendCapacityQuery, hostgroup)
	if err != nil {
		return snapshot, fmt.Errorf("writer connections: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		backend := capacityBackend{}
		if err := rows.Scan(&backend.host, &backend.port, &backend.status, &backend.used); err != nil {
			return snapshot, err
		}
		if backend.host == "" || backend.port < 0 || backend.port > 65535 || backend.used < 0 {
			return snapshot, fmt.Errorf("invalid writer connection counters or endpoint")
		}
		snapshot.backends = append(snapshot.backends, backend)
	}
	return snapshot, rows.Err()
}
