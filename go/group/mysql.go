// Provide a MySQL backend as alternative to raft consensus

// Expect the following backend tables:

/*
CREATE TABLE service_election (
  domain varchar(32) NOT NULL,
  service_id varchar(128) NOT NULL,
  last_seen_active timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (domain)
);

CREATE TABLE throttled_apps (
  app_name varchar(128) NOT NULL,
	throttled_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL,
	ratio DOUBLE,
  PRIMARY KEY (app_name)
);
*/

package group

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/config"
	"github.com/github/freno/go/throttle"

	"github.com/outbrain/golib/log"
	"github.com/outbrain/golib/sqlutils"
	metrics "github.com/rcrowley/go-metrics"
)

type MySQLBackend struct {
	db          *sql.DB
	domain      string
	serviceId   string
	leaderState int64
	throttler   *throttle.Throttler
}

const maxConnections = 3

func NewMySQLBackend(throttler *throttle.Throttler) (*MySQLBackend, error) {
	if config.Settings().BackendMySQLHost == "" {
		return nil, nil
	}
	dbUri := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&charset=utf8mb4,utf8,latin1&timeout=500ms",
		config.Settings().BackendMySQLUser, config.Settings().BackendMySQLPassword, config.Settings().BackendMySQLHost, config.Settings().BackendMySQLPort, config.Settings().BackendMySQLSchema,
	)
	db, _, err := sqlutils.GetDB(dbUri)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(maxConnections)
	log.Debugf("created db at: %s", dbUri)
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	backend := &MySQLBackend{
		db:        db,
		domain:    fmt.Sprintf("%s:%s", config.Settings().DataCenter, config.Settings().Environment),
		serviceId: hostname,
		throttler: throttler,
	}
	go backend.continuousElections()
	return backend, nil
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// Monitor is a utility function to routinely observe leadership state.
// It doesn't actually do much; merely takes notes.
func (backend *MySQLBackend) continuousElections() {
	t := time.NewTicker(1 * time.Second)

	for range t.C {
		err := backend.AttemptLeadership()
		log.Errore(err)

		newLeaderState, err := backend.ReadLeadership()
		if err == nil {
			if newLeaderState != backend.leaderState {
				backend.onLeaderStateChange(newLeaderState)
				atomic.StoreInt64(&backend.leaderState, newLeaderState)
			}
		} else {
			// maintain state: graceful response to backend errors
			log.Errore(err)
		}
		go metrics.GetOrRegisterGauge("backend.mysql.is_leader", nil).Update(atomic.LoadInt64(&backend.leaderState))
		go metrics.GetOrRegisterGauge("backend.mysql.is_healthy", nil).Update(boolToInt64(err == nil))
	}
}

func (backend *MySQLBackend) onLeaderStateChange(newLeaderState int64) error {
	if newLeaderState > 0 {
		log.Infof("Transitioned into leader state")
		backend.expireThrottledApps()
		backend.readThrottledApps()
	} else {
		log.Infof("Transitioned out of leader state")
	}
	return nil
}

func (backend *MySQLBackend) IsLeader() bool {
	return atomic.LoadInt64(&backend.leaderState) > 0
}

func (backend *MySQLBackend) AttemptLeadership() error {
	query := `
    insert ignore into service_election (
        domain, service_id, last_seen_active
      ) values (
        ?, ?, now()
      ) on duplicate key update
      service_id = if(last_seen_active < now() - interval 20 second, values(service_id), service_id),
      last_seen_active = if(service_id = values(service_id), values(last_seen_active), last_seen_active)
  `
	args := sqlutils.Args(backend.domain, backend.serviceId)
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	return err
}

func (backend *MySQLBackend) ForceLeadership() error {
	query := `
    replace into service_election (
        domain, service_id, last_seen_active
      ) values (
        ?, ?, now()
      )
  `
	args := sqlutils.Args(backend.domain, backend.serviceId)
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	return err
}

func (backend *MySQLBackend) Reelect() error {
	query := `
    delete from service_election where domain=?
  `
	args := sqlutils.Args(backend.domain)
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	return err
}

func (backend *MySQLBackend) ReadLeadership() (int64, error) {
	query := `
    select count(*) > 0
      from service_election
      where domain=?
      and service_id=?
  `
	args := sqlutils.Args(backend.domain, backend.serviceId)

	var count int64
	err := backend.db.QueryRow(query, args...).Scan(&count)

	return count, err
}

func (backend *MySQLBackend) expireThrottledApps() error {
	query := `delete from throttled_apps where expires_at <= now()`
	_, err := sqlutils.ExecNoPrepare(backend.db, query)
	return err
}

func (backend *MySQLBackend) readThrottledApps() error {
	query := `
		select
			app_name,
			timestampdiff(second, now(), expires_at) as ttl_seconds,
			ratio
		from
			throttled_apps
		where
			expires_at > now()
	`

	err := sqlutils.QueryRowsMap(backend.db, query, func(m sqlutils.RowMap) error {
		appName := m.GetString("app_name")
		ttlSeconds := m.GetInt64("ttl_seconds")
		ratio, _ := strconv.ParseFloat(m.GetString("ratio"), 64)
		expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)

		go log.Debugf("read-throttled-apps: app=%s, ttlSeconds%+v, expiresAt=%+v, ratio=%+v", appName, ttlSeconds, expiresAt, ratio)
		go backend.throttler.ThrottleApp(appName, expiresAt, ratio)
		return nil
	})

	return err
}

func (backend *MySQLBackend) ThrottleApp(appName string, ttlMinutes int64, expireAt time.Time, ratio float64) error {
	log.Debugf("throttle-app: app=%s, ttlMinutes=%+v, expireAt=%+v, ratio=%+v", appName, ttlMinutes, expireAt, ratio)
	var query string
	var args []interface{}
	if ttlMinutes > 0 {
		query = `
	    replace into throttled_apps (
	        app_name, throttled_at, expires_at, ratio
	      ) values (
	        ?, now(), now() + interval ? minute, ?
	      )
	  `
		args = sqlutils.Args(appName, ttlMinutes, ratio)
	} else {
		// TTL=0 ; if app is already throttled, keep existing TTL and only update ratio.
		// if app does not exist use DefaultThrottleTTL
		query = `
	    insert into throttled_apps (
	        app_name, throttled_at, expires_at, ratio
	      ) values (
	        ?, now(), now() + interval ? minute, ?
	      )
			on duplicate key update
				ratio=values(ratio)
		`
		args = sqlutils.Args(appName, throttle.DefaultThrottleTTLMinutes, ratio)
	}
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	backend.throttler.ThrottleApp(appName, expireAt, ratio)
	return err
}

func (backend *MySQLBackend) ThrottledAppsMap() (result map[string](*base.AppThrottle)) {
	return backend.throttler.ThrottledAppsMap()
}

func (backend *MySQLBackend) UnthrottleApp(appName string) error {
	backend.throttler.UnthrottleApp(appName)
	query := `
    delete from throttled_apps where app_name=?
  `
	args := sqlutils.Args(appName)
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	return err
}

func (backend *MySQLBackend) RecentAppsMap() (result map[string](*base.RecentApp)) {
	return backend.throttler.RecentAppsMap()
}
