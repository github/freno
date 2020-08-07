// Provide a MySQL backend as alternative to raft consensus

// Expect the following backend tables:

/*
CREATE TABLE service_election (
  domain varchar(32) NOT NULL,
  share_domain varchar(32) NOT NULL,
  service_id varchar(128) NOT NULL,
  last_seen_active timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (domain),
  KEY share_domain_idx (share_domain,last_seen_active)
);

CREATE TABLE service_health (
service_id varchar(128) NOT NULL,
  domain varchar(32) NOT NULL,
  share_domain varchar(32) NOT NULL,
  last_seen_active timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (service_id),
  KEY last_seen_active_idx (last_seen_active)
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

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/throttle"

	"github.com/outbrain/golib/log"
	"github.com/outbrain/golib/sqlutils"
	metrics "github.com/rcrowley/go-metrics"
)

type MySQLBackend struct {
	db          *sql.DB
	domain      string
	shareDomain string
	serviceId   string
	leaderState int64
	healthState int64
	throttler   *throttle.Throttler
}

const maxConnections = 3
const electionExpireSeconds = 5

const electionInterval = time.Second
const healthInterval = 2 * electionInterval
const stateInterval = 10 * time.Second

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
	domain := config.Settings().Domain
	if domain == "" {
		domain = fmt.Sprintf("%s:%s", config.Settings().DataCenter, config.Settings().Environment)
	}
	shareDomain := config.Settings().ShareDomain
	serviceId := fmt.Sprintf("%s:%d", hostname, config.Settings().ListenPort)
	backend := &MySQLBackend{
		db:          db,
		domain:      domain,
		shareDomain: shareDomain,
		serviceId:   serviceId,
		throttler:   throttler,
	}
	go backend.continuousOperations()
	return backend, nil
}

// Monitor is a utility function to routinely observe leadership state.
// It doesn't actually do much; merely takes notes.
func (backend *MySQLBackend) continuousOperations() {
	healthTicker := time.NewTicker(healthInterval)
	electionsTicker := time.NewTicker(electionInterval)
	stateTicker := time.NewTicker(stateInterval)

	for {
		select {
		case <-healthTicker.C:
			{
				err := backend.RegisterHealth()
				log.Errore(err)
			}
		case <-electionsTicker.C:
			{
				err := backend.AttemptLeadership()
				log.Errore(err)

				newLeaderState, _, err := backend.ReadLeadership()
				if err == nil {
					atomic.StoreInt64(&backend.healthState, 1)
					if newLeaderState != backend.leaderState {
						backend.onLeaderStateChange(newLeaderState)
						atomic.StoreInt64(&backend.leaderState, newLeaderState)
					}
				} else {
					atomic.StoreInt64(&backend.healthState, 0)
					// and maintain leader state: graceful response to backend errors
					log.Errore(err)
				}
			}
		case <-stateTicker.C:
			{
				backend.readThrottledApps()
			}
		}
	}
}

func (backend *MySQLBackend) onLeaderStateChange(newLeaderState int64) error {
	if newLeaderState > 0 {
		log.Infof("Transitioned into leader state")
		backend.readThrottledApps()
	} else {
		log.Infof("Transitioned out of leader state")
	}
	return nil
}

func (backend *MySQLBackend) IsHealthy() bool {
	return atomic.LoadInt64(&backend.healthState) > 0
}

func (backend *MySQLBackend) IsLeader() bool {
	if ForceLeadership {
		return true
	}
	return atomic.LoadInt64(&backend.leaderState) > 0
}

func (backend *MySQLBackend) GetLeader() string {
	_, leader, _ := backend.ReadLeadership()
	return leader
}

func (backend *MySQLBackend) GetStateDescription() string {
	if atomic.LoadInt64(&backend.leaderState) > 0 {
		return "Leader"
	}
	if atomic.LoadInt64(&backend.healthState) > 0 {
		return "Healthy"
	}
	return "Unhealthy"
}

func (backend *MySQLBackend) GetStatus() *ConsensusServiceStatus {
	shareDomainServicesList := []string{backend.serviceId}
	shareDomainServices, _ := backend.GetSharedDomainServices()
	for _, service := range shareDomainServices {
		shareDomainServicesList = append(shareDomainServicesList, service)
	}
	healthyDomainServicesList, _ := backend.GetHealthyDomainServices()
	return &ConsensusServiceStatus{
		ServiceID:                 backend.serviceId,
		Healthy:                   backend.IsHealthy(),
		IsLeader:                  backend.IsLeader(),
		Leader:                    backend.GetLeader(),
		State:                     backend.GetStateDescription(),
		Domain:                    backend.domain,
		ShareDomain:               backend.shareDomain,
		ShareDomainServices:       shareDomainServices,
		ShareDomainServicesList:   shareDomainServicesList,
		HealthyDomainServicesList: healthyDomainServicesList,
	}
}

func (backend *MySQLBackend) RegisterHealth() error {
	query := `
    insert ignore into service_health (
        service_id, domain, share_domain, last_seen_active
      ) values (
        ?, ?, ?, now()
      ) on duplicate key update
			domain       = values(domain),
      share_domain = values(share_domain),
      last_seen_active = values(last_seen_active)
  `
	args := sqlutils.Args(backend.serviceId, backend.domain, backend.shareDomain)
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	return err
}

// GetHealthyDomainServices returns list of services healthy within same domain as this service,
// including this service
func (backend *MySQLBackend) GetHealthyDomainServices() (services []string, err error) {
	query := `
		select
			service_id
		from
			service_health
		where
			domain = ?
			and last_seen_active >= now() - interval ? second
	`
	args := sqlutils.Args(backend.domain, electionExpireSeconds)
	err = sqlutils.QueryRowsMap(backend.db, query, func(m sqlutils.RowMap) error {
		services = append(services, m.GetString("service_id"))
		return nil
	}, args...)

	return services, err
}

func (backend *MySQLBackend) AttemptLeadership() error {
	query := `
    insert ignore into service_election (
        domain, share_domain, service_id, last_seen_active
      ) values (
        ?, ?, ?, now()
      ) on duplicate key update
			service_id       = if(last_seen_active < now() - interval ? second, values(service_id), service_id),
      share_domain     = if(service_id = values(service_id), values(share_domain), share_domain),
      last_seen_active = if(service_id = values(service_id), values(last_seen_active), last_seen_active)
  `
	args := sqlutils.Args(backend.domain, backend.shareDomain, backend.serviceId, electionExpireSeconds)
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

func (backend *MySQLBackend) ReadLeadership() (leaderState int64, leader string, err error) {
	query := `
    select
				ifnull(max(service_id) = ?, 0) as is_leader,
				ifnull(max(service_id), '') as service_id
      from
				service_election
      where
				domain=?
  `
	args := sqlutils.Args(backend.serviceId, backend.domain)

	err = backend.db.QueryRow(query, args...).Scan(&leaderState, &leader)

	log.Debugf("read-leadership: leaderState=%+v, leader=%+v, domain=%s, err=%+v", leaderState, leader, backend.domain, err)
	return leaderState, leader, err
}

// GetSharedDomainServices returns active leader services that have same ShareDomain as this service:
// - assuming ShareDomain is not empty
// - excluding this very service
func (backend *MySQLBackend) GetSharedDomainServices() (services map[string]string, err error) {
	if backend.shareDomain == "" {
		return services, err
	}
	services = make(map[string]string)
	query := `
		select
			domain,
			service_id
		from
			service_election
		where
			share_domain = ?
			and last_seen_active >= now() - interval ? second
			and service_id != ?
	`
	args := sqlutils.Args(backend.shareDomain, electionExpireSeconds, backend.serviceId)
	err = sqlutils.QueryRowsMap(backend.db, query, func(m sqlutils.RowMap) error {
		services[m.GetString("domain")] = m.GetString("service_id")
		return nil
	}, args...)

	return services, err
}

func (backend *MySQLBackend) readThrottledApps() error {
	query := `
		select
			app_name,
			timestampdiff(second, now(), expires_at) as ttl_seconds,
			ratio
		from
			throttled_apps
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
    update throttled_apps set expires_at=now() where app_name=?
  `
	args := sqlutils.Args(appName)
	_, err := sqlutils.ExecNoPrepare(backend.db, query, args...)
	return err
}

func (backend *MySQLBackend) RecentAppsMap() (result map[string](*base.RecentApp)) {
	return backend.throttler.RecentAppsMap()
}

func (backend *MySQLBackend) Monitor() {
	t := time.NewTicker(monitorInterval)
	for range t.C {
		go metrics.GetOrRegisterGauge("backend.mysql.is_leader", nil).Update(atomic.LoadInt64(&backend.leaderState))
		go metrics.GetOrRegisterGauge("backend.mysql.is_healthy", nil).Update(atomic.LoadInt64(&backend.healthState))
	}
}
