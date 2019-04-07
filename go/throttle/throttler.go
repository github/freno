package throttle

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/config"
	"github.com/github/freno/go/haproxy"
	"github.com/github/freno/go/mysql"
	"github.com/github/freno/go/vitess"

	"github.com/outbrain/golib/log"
	"github.com/patrickmn/go-cache"

	"github.com/bradfitz/gomemcache/memcache"
	metrics "github.com/rcrowley/go-metrics"
)

const leaderCheckInterval = 1 * time.Second
const mysqlCollectInterval = 100 * time.Millisecond
const mysqlRefreshInterval = 10 * time.Second
const mysqlAggreateInterval = 50 * time.Millisecond
const mysqlHttpCheckInterval = 5 * time.Second

const aggregatedMetricsExpiration = 5 * time.Second
const aggregatedMetricsCleanup = 1 * time.Second
const throttledAppsSnapshotInterval = 5 * time.Second
const recentAppsExpiration = time.Hour * 24

const DefaultThrottleTTLMinutes = 60
const DefaultThrottleRatio = 1.0

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Throttler struct {
	isLeader     bool
	isLeaderFunc func() bool

	mysqlThrottleMetricChan chan *mysql.MySQLThrottleMetric
	mysqlHttpCheckChan      chan *mysql.MySQLHttpCheck
	mysqlInventoryChan      chan *mysql.MySQLInventory
	mysqlClusterProbesChan  chan *mysql.ClusterProbes

	mysqlInventory *mysql.MySQLInventory

	mysqlClusterThresholds *cache.Cache
	aggregatedMetrics      *cache.Cache
	throttledApps          *cache.Cache
	recentApps             *cache.Cache
	metricsHealth          *cache.Cache

	memcacheClient *memcache.Client
	memcachePath   string

	throttledAppsMutex sync.Mutex
}

func NewThrottler() *Throttler {
	throttler := &Throttler{
		isLeader: false,

		mysqlThrottleMetricChan: make(chan *mysql.MySQLThrottleMetric),
		mysqlHttpCheckChan:      make(chan *mysql.MySQLHttpCheck),

		mysqlInventoryChan:     make(chan *mysql.MySQLInventory, 1),
		mysqlClusterProbesChan: make(chan *mysql.ClusterProbes),
		mysqlInventory:         mysql.NewMySQLInventory(),

		throttledApps:          cache.New(cache.NoExpiration, 10*time.Second),
		mysqlClusterThresholds: cache.New(cache.NoExpiration, 0),
		aggregatedMetrics:      cache.New(aggregatedMetricsExpiration, aggregatedMetricsCleanup),
		recentApps:             cache.New(recentAppsExpiration, time.Minute),
		metricsHealth:          cache.New(cache.NoExpiration, 0),
	}
	throttler.ThrottleApp("abusing-app", time.Now().Add(time.Hour*24*365*10), DefaultThrottleRatio)
	if memcacheServers := config.Settings().MemcacheServers; len(memcacheServers) > 0 {
		throttler.memcacheClient = memcache.New(memcacheServers...)
	}
	throttler.memcachePath = config.Settings().MemcachePath

	return throttler
}

func (throttler *Throttler) SetLeaderFunc(isLeaderFunc func() bool) {
	throttler.isLeaderFunc = isLeaderFunc
}

func (throttler *Throttler) ThrottledAppsSnapshot() map[string]cache.Item {
	return throttler.throttledApps.Items()
}

func (throttler *Throttler) Operate() {
	leaderCheckTick := time.Tick(leaderCheckInterval)
	mysqlCollectTick := time.Tick(mysqlCollectInterval)
	mysqlRefreshTick := time.Tick(mysqlRefreshInterval)
	mysqlAggregateTick := time.Tick(mysqlAggreateInterval)
	mysqlHttpCheckTick := time.Tick(mysqlHttpCheckInterval)
	throttledAppsTick := time.Tick(throttledAppsSnapshotInterval)

	// initial read of inventory:
	go throttler.refreshMySQLInventory()

	for {
		select {
		case <-leaderCheckTick:
			{
				// sparse
				throttler.isLeader = throttler.isLeaderFunc()
			}
		case <-mysqlCollectTick:
			{
				// frequent
				throttler.collectMySQLMetrics()
			}
		case <-mysqlHttpCheckTick:
			{
				throttler.collectMySQLHttpChecks()
			}
		case metric := <-throttler.mysqlThrottleMetricChan:
			{
				// incoming MySQL metric, frequent, as result of collectMySQLMetrics()
				throttler.mysqlInventory.InstanceKeyMetrics[metric.GetClusterInstanceKey()] = metric
			}
		case httpCheckResult := <-throttler.mysqlHttpCheckChan:
			{
				// incoming MySQL metric, frequent, as result of collectMySQLMetrics()
				throttler.mysqlInventory.ClusterInstanceHttpChecks[httpCheckResult.HashKey()] = httpCheckResult.CheckResult
			}
		case <-mysqlRefreshTick:
			{
				// sparse
				go throttler.refreshMySQLInventory()
			}
		case probes := <-throttler.mysqlClusterProbesChan:
			{
				// incoming structural update, sparse, as result of refreshMySQLInventory()
				throttler.updateMySQLClusterProbes(probes)
			}
		case <-mysqlAggregateTick:
			{
				throttler.aggregateMySQLMetrics()
			}
		case <-throttledAppsTick:
			{
				go throttler.expireThrottledApps()
				go throttler.pushStatusToExpVar()
			}
		}
		if !throttler.isLeader {
			time.Sleep(1 * time.Second)
		}
	}
}

func (throttler *Throttler) collectMySQLMetrics() error {
	if !throttler.isLeader {
		return nil
	}
	// synchronously, get lists of probes
	for clusterName, probes := range throttler.mysqlInventory.ClustersProbes {
		clusterName := clusterName
		probes := probes
		go func() {
			// probes is known not to change. It can be *replaced*, but not changed.
			// so it's safe to iterate it
			for _, probe := range *probes {
				probe := probe
				go func() {
					// Avoid querying the same server twice at the same time. If previous read is still there,
					// we avoid re-reading it.
					if !atomic.CompareAndSwapInt64(&probe.QueryInProgress, 0, 1) {
						return
					}
					defer atomic.StoreInt64(&probe.QueryInProgress, 0)
					throttleMetrics := mysql.ReadThrottleMetric(probe, clusterName)
					throttler.mysqlThrottleMetricChan <- throttleMetrics
				}()
			}
		}()
	}
	return nil
}

func (throttler *Throttler) collectMySQLHttpChecks() error {
	if !throttler.isLeader {
		return nil
	}
	// synchronously, get lists of probes
	for clusterName, probes := range throttler.mysqlInventory.ClustersProbes {
		clusterName := clusterName
		probes := probes
		go func() {
			// probes is known not to change. It can be *replaced*, but not changed.
			// so it's safe to iterate it
			for _, probe := range *probes {
				probe := probe
				go func() {
					// Avoid querying the same server twice at the same time. If previous read is still there,
					// we avoid re-reading it.
					if !atomic.CompareAndSwapInt64(&probe.HttpCheckInProgress, 0, 1) {
						return
					}
					defer atomic.StoreInt64(&probe.HttpCheckInProgress, 0)
					httpCheckResult := mysql.CheckHttp(clusterName, probe)
					throttler.mysqlHttpCheckChan <- httpCheckResult
				}()
			}
		}()
	}
	return nil
}

// refreshMySQLInventory will re-structure the inventory based on reading config settings, and potentially
// re-querying dynamic data such as HAProxy list of hosts
func (throttler *Throttler) refreshMySQLInventory() error {
	if !throttler.isLeader {
		return nil
	}
	log.Debugf("refreshing MySQL inventory")

	addInstanceKey := func(key *mysql.InstanceKey, clusterSettings *config.MySQLClusterConfigurationSettings, probes *mysql.Probes) {
		for _, ignore := range clusterSettings.IgnoreHosts {
			if strings.Contains(key.DisplayString(), ignore) {
				log.Debugf("instance key ignored: %+v", key)
				return
			}
		}
		log.Debugf("read instance key: %+v", key)

		probe := &mysql.Probe{
			Key:           *key,
			User:          clusterSettings.User,
			Password:      clusterSettings.Password,
			MetricQuery:   clusterSettings.MetricQuery,
			CacheMillis:   clusterSettings.CacheMillis,
			HttpCheckPath: clusterSettings.HttpCheckPath,
			HttpCheckPort: clusterSettings.HttpCheckPort,
		}
		(*probes)[*key] = probe
	}

	for clusterName, clusterSettings := range config.Settings().Stores.MySQL.Clusters {
		clusterName := clusterName
		clusterSettings := clusterSettings
		// config may dynamically change, but internal structure (config.Settings().Stores.MySQL.Clusters in our case)
		// is immutable and can only be _replaced_. Hence, it's safe to read in a goroutine:
		go func() error {
			throttler.mysqlClusterThresholds.Set(clusterName, clusterSettings.ThrottleThreshold, cache.DefaultExpiration)
			if !clusterSettings.HAProxySettings.IsEmpty() {
				poolName := clusterSettings.HAProxySettings.PoolName
				totalHosts := []string{}
				for _, hostPort := range clusterSettings.HAProxySettings.GetProxyAddresses() {
					log.Debugf("getting haproxy data from %s:%d", hostPort.Host, hostPort.Port)
					csv, err := haproxy.Read(hostPort.Host, hostPort.Port)
					if err != nil {
						return log.Errorf("Unable to get HAproxy data from %s:%d: %+v", hostPort.Host, hostPort, err)
					}
					if hosts, err := haproxy.ParseCsvHosts(csv, poolName); err == nil {
						totalHosts = append(totalHosts, hosts...)
						log.Debugf("Read %+v hosts from haproxy %s:%d/#%s", len(hosts), hostPort.Host, hostPort.Port, poolName)
					} else {
						log.Errorf("Unable to get HAproxy hosts from %s:%d/#%s: %+v", hostPort.Host, hostPort.Port, poolName, err)
					}
				}
				if len(totalHosts) == 0 {
					return log.Errorf("Unable to get any HAproxy hosts for pool: %+v", poolName)
				}
				clusterProbes := &mysql.ClusterProbes{
					ClusterName:          clusterName,
					IgnoreHostsCount:     clusterSettings.IgnoreHostsCount,
					IgnoreHostsThreshold: clusterSettings.IgnoreHostsThreshold,
					InstanceProbes:       mysql.NewProbes(),
				}
				for _, host := range totalHosts {
					key := mysql.InstanceKey{Hostname: host, Port: clusterSettings.Port}
					addInstanceKey(&key, clusterSettings, clusterProbes.InstanceProbes)
				}
				throttler.mysqlClusterProbesChan <- clusterProbes
				return nil
			}

			if !clusterSettings.VitessSettings.IsEmpty() {
				log.Debugf("getting vitess data from %s", clusterSettings.VitessSettings.API)
				keyspace := clusterSettings.VitessSettings.Keyspace
				shard := clusterSettings.VitessSettings.Shard
				tablets, err := vitess.ParseTablets(clusterSettings.VitessSettings.API, keyspace, shard)
				if err != nil {
					return log.Errorf("Unable to get vitess hosts from %s, %s/%s: %+v", clusterSettings.VitessSettings.API, keyspace, shard, err)
				}
				log.Debugf("Read %+v hosts from vitess %s, %s/%s", len(tablets), clusterSettings.VitessSettings.API, keyspace, shard)
				clusterProbes := &mysql.ClusterProbes{
					ClusterName:      clusterName,
					IgnoreHostsCount: clusterSettings.IgnoreHostsCount,
					InstanceProbes:   mysql.NewProbes(),
				}
				for _, tablet := range tablets {
					key := mysql.InstanceKey{Hostname: tablet.MysqlHostname, Port: int(tablet.MysqlPort)}
					addInstanceKey(&key, clusterSettings, clusterProbes.InstanceProbes)
				}
				throttler.mysqlClusterProbesChan <- clusterProbes
				return nil
			}

			if !clusterSettings.StaticHostsSettings.IsEmpty() {
				clusterProbes := &mysql.ClusterProbes{
					ClusterName:    clusterName,
					InstanceProbes: mysql.NewProbes(),
				}
				for _, host := range clusterSettings.StaticHostsSettings.Hosts {
					key, err := mysql.ParseInstanceKey(host, clusterSettings.Port)
					if err != nil {
						return log.Errore(err)
					}
					addInstanceKey(key, clusterSettings, clusterProbes.InstanceProbes)
				}
				throttler.mysqlClusterProbesChan <- clusterProbes
				return nil
			}
			return log.Errorf("Could not find any hosts definition for cluster %s", clusterName)
		}()
	}
	return nil
}

// synchronous update of inventory
func (throttler *Throttler) updateMySQLClusterProbes(clusterProbes *mysql.ClusterProbes) error {
	log.Debugf("onMySQLClusterProbes: %s", clusterProbes.ClusterName)
	throttler.mysqlInventory.ClustersProbes[clusterProbes.ClusterName] = clusterProbes.InstanceProbes
	throttler.mysqlInventory.IgnoreHostsCount[clusterProbes.ClusterName] = clusterProbes.IgnoreHostsCount
	throttler.mysqlInventory.IgnoreHostsThreshold[clusterProbes.ClusterName] = clusterProbes.IgnoreHostsThreshold
	return nil
}

// synchronous aggregation of collected data
func (throttler *Throttler) aggregateMySQLMetrics() error {
	if !throttler.isLeader {
		return nil
	}
	for clusterName, probes := range throttler.mysqlInventory.ClustersProbes {
		metricName := fmt.Sprintf("mysql/%s", clusterName)
		ignoreHostsCount := throttler.mysqlInventory.IgnoreHostsCount[clusterName]
		ignoreHostsThreshold := throttler.mysqlInventory.IgnoreHostsThreshold[clusterName]
		aggregatedMetric := aggregateMySQLProbes(probes, clusterName, throttler.mysqlInventory.InstanceKeyMetrics, throttler.mysqlInventory.ClusterInstanceHttpChecks, ignoreHostsCount, config.Settings().Stores.MySQL.IgnoreDialTcpErrors, ignoreHostsThreshold)
		go throttler.aggregatedMetrics.Set(metricName, aggregatedMetric, cache.DefaultExpiration)
		if throttler.memcacheClient != nil {
			go func() {
				memcacheKey := fmt.Sprintf("%s/%s", throttler.memcachePath, metricName)
				value, err := aggregatedMetric.Get()
				if err != nil {
					throttler.memcacheClient.Delete(memcacheKey)
				} else {
					epochMillis := time.Now().UnixNano() / 1000000
					entryVal := fmt.Sprintf("%d:%.6f", epochMillis, value)
					throttler.memcacheClient.Set(&memcache.Item{Key: memcacheKey, Value: []byte(entryVal), Expiration: 1})
				}
			}()
		}
	}
	return nil
}

func (throttler *Throttler) pushStatusToExpVar() {
	metrics.DefaultRegistry.Each(func(metricName string, _ interface{}) {
		if strings.HasPrefix(metricName, "throttled_states.") {
			metrics.Get(metricName).(metrics.Gauge).Update(0)
		}
	})

	for appName := range throttler.ThrottledAppsSnapshot() {
		metrics.GetOrRegisterGauge(fmt.Sprintf("throttled_states.%s", appName), nil).Update(1)
	}
}

func (throttler *Throttler) getNamedMetric(metricName string) base.MetricResult {
	if metricResultVal, found := throttler.aggregatedMetrics.Get(metricName); found {
		return metricResultVal.(base.MetricResult)
	}
	return base.NoSuchMetric
}

func (throttler *Throttler) getMySQLClusterMetrics(clusterName string) (base.MetricResult, float64) {
	if thresholdVal, found := throttler.mysqlClusterThresholds.Get(clusterName); found {
		threshold, _ := thresholdVal.(float64)
		metricName := fmt.Sprintf("mysql/%s", clusterName)
		return throttler.getNamedMetric(metricName), threshold
	}

	return base.NoSuchMetric, 0
}

func (throttler *Throttler) aggregatedMetricsSnapshot() map[string]base.MetricResult {
	snapshot := make(map[string]base.MetricResult)
	for key, value := range throttler.aggregatedMetrics.Items() {
		metricResult, _ := value.Object.(base.MetricResult)
		snapshot[key] = metricResult
	}
	return snapshot
}

func (throttler *Throttler) expireThrottledApps() {
	now := time.Now()
	for appName, item := range throttler.throttledApps.Items() {
		appThrottle := item.Object.(*base.AppThrottle)
		if appThrottle.ExpireAt.Before(now) {
			throttler.UnthrottleApp(appName)
		}
	}
}

func (throttler *Throttler) ThrottleApp(appName string, expireAt time.Time, ratio float64) {
	throttler.throttledAppsMutex.Lock()
	defer throttler.throttledAppsMutex.Unlock()

	var appThrottle *base.AppThrottle
	now := time.Now()
	if object, found := throttler.throttledApps.Get(appName); found {
		appThrottle = object.(*base.AppThrottle)
		if !expireAt.IsZero() {
			appThrottle.ExpireAt = expireAt
		}
		if ratio >= 0 {
			appThrottle.Ratio = ratio
		}
	} else {
		if expireAt.IsZero() {
			expireAt = now.Add(DefaultThrottleTTLMinutes * time.Minute)
		}
		if ratio < 0 {
			ratio = DefaultThrottleRatio
		}
		appThrottle = base.NewAppThrottle(expireAt, ratio)
	}
	if appThrottle.ExpireAt.Before(now) {
		throttler.UnthrottleApp(appName)
	} else {
		throttler.throttledApps.Set(appName, appThrottle, cache.DefaultExpiration)
	}
}

func (throttler *Throttler) UnthrottleApp(appName string) {
	throttler.throttledApps.Delete(appName)
}

func (throttler *Throttler) IsAppThrottled(appName string) bool {
	if object, found := throttler.throttledApps.Get(appName); found {
		appThrottle := object.(*base.AppThrottle)
		if appThrottle.ExpireAt.Before(time.Now()) {
			// throttling cleanup hasn't purged yet, but it is expired
			return false
		}
		// handle ratio
		if rand.Float64() < appThrottle.Ratio {
			return true
		}
	}
	return false
}

func (throttler *Throttler) ThrottledAppsMap() (result map[string](*base.AppThrottle)) {
	result = make(map[string](*base.AppThrottle))

	for appName, item := range throttler.throttledApps.Items() {
		appThrottle := item.Object.(*base.AppThrottle)
		result[appName] = appThrottle
	}
	return result
}

func (throttler *Throttler) markRecentApp(appName string, remoteAddr string) {
	recentAppKey := fmt.Sprintf("%s/%s", appName, remoteAddr)
	throttler.recentApps.Set(recentAppKey, time.Now(), cache.DefaultExpiration)
}

func (throttler *Throttler) RecentAppsMap() (result map[string](*base.RecentApp)) {
	result = make(map[string](*base.RecentApp))

	for recentAppKey, item := range throttler.recentApps.Items() {
		recentApp := base.NewRecentApp(item.Object.(time.Time))
		result[recentAppKey] = recentApp
	}
	return result
}

// markMetricHealthy will mark the time "now" as the last time a given metric was checked to be "OK"
func (throttler *Throttler) markMetricHealthy(metricName string) {
	throttler.metricsHealth.Set(metricName, time.Now(), cache.DefaultExpiration)
}

// timeSinceMetricHealthy returns time elapsed since the last time a metric checked "OK"
func (throttler *Throttler) timeSinceMetricHealthy(metricName string) (timeSinceHealthy time.Duration, found bool) {
	if lastOKTime, found := throttler.metricsHealth.Get(metricName); found {
		return time.Since(lastOKTime.(time.Time)), true
	}
	return 0, false
}

func (throttler *Throttler) metricsHealthSnapshot() map[string](*base.MetricHealth) {
	snapshot := make(map[string](*base.MetricHealth))
	for key, value := range throttler.metricsHealth.Items() {
		lastHealthyAt, _ := value.Object.(time.Time)
		snapshot[key] = base.NewMetricHealth(lastHealthyAt)
	}
	return snapshot
}

func (throttler *Throttler) AppRequestMetricResult(appName string, metricResultFunc base.MetricResultFunc) (metricResult base.MetricResult, threshold float64) {
	if throttler.IsAppThrottled(appName) {
		return base.AppDeniedMetric, 0
	}
	return metricResultFunc()
}
