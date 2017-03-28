package throttle

import (
	"expvar"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/config"
	"github.com/github/freno/go/haproxy"
	"github.com/github/freno/go/mysql"

	"github.com/outbrain/golib/log"
	"github.com/patrickmn/go-cache"
)

const leaderCheckInterval = 1 * time.Second
const mysqlCollectInterval = 100 * time.Millisecond
const mysqlRefreshInterval = 10 * time.Second
const mysqlAggreateInterval = 50 * time.Millisecond

const aggregatedMetricsExpiration = 5 * time.Second
const aggregatedMetricsCleanup = 1 * time.Second
const throttledAppsSnapshotInterval = 5 * time.Second

var throttledAppsExpVar = expvar.NewMap("throttled.apps")

type Throttler struct {
	isLeader     bool
	isLeaderFunc func() bool

	mysqlThrottleMetricChan chan *mysql.MySQLThrottleMetric
	mysqlInventoryChan      chan *mysql.MySQLInventory
	mysqlClusterProbesChan  chan *mysql.ClusterProbes

	mysqlInventory *mysql.MySQLInventory

	mysqlClusterThresholds *cache.Cache
	aggregatedMetrics      *cache.Cache
	throttledApps          *cache.Cache
}

func NewThrottler(isLeaderFunc func() bool) *Throttler {
	throttler := &Throttler{
		isLeader:     false,
		isLeaderFunc: isLeaderFunc,

		mysqlThrottleMetricChan: make(chan *mysql.MySQLThrottleMetric),

		mysqlInventoryChan:     make(chan *mysql.MySQLInventory, 1),
		mysqlClusterProbesChan: make(chan *mysql.ClusterProbes),
		mysqlInventory:         mysql.NewMySQLInventory(),

		throttledApps:          cache.New(cache.NoExpiration, 0),
		mysqlClusterThresholds: cache.New(cache.NoExpiration, 0),
		aggregatedMetrics:      cache.New(aggregatedMetricsExpiration, aggregatedMetricsCleanup),
	}
	throttler.ThrottleApp("abusing-app")
	return throttler
}

func (throttler *Throttler) ThrottledAppsSnapshot() map[string]cache.Item {
	return throttler.throttledApps.Items()
}

func (throttler *Throttler) Operate() {
	leaderCheckTick := time.Tick(leaderCheckInterval)
	mysqlCollectTick := time.Tick(mysqlCollectInterval)
	mysqlRefreshTick := time.Tick(mysqlRefreshInterval)
	mysqlAggregateTick := time.Tick(mysqlAggreateInterval)
	throttledAppsTick := time.Tick(throttledAppsSnapshotInterval)
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
		case metric := <-throttler.mysqlThrottleMetricChan:
			{
				// incoming MySQL metric, frequent, as result of collectMySQLMetrics()
				throttler.mysqlInventory.InstanceKeyMetrics[metric.Key] = metric
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
	for _, probes := range throttler.mysqlInventory.ClustersProbes {
		probes := probes
		go func() {
			// probes is known not to change. It can be *replaced*, but not changed.
			// so it's safe to iterate it
			for _, probe := range *probes {
				probe := probe
				go func() {
					// Avoid querying the same server twice at the same time. If previous read is still there,
					// we avoid re-reading it.
					if !atomic.CompareAndSwapInt64(&probe.InProgress, 0, 1) {
						return
					}
					defer atomic.StoreInt64(&probe.InProgress, 0)
					throttleMetrics := mysql.ReadThrottleMetric(probe)
					throttler.mysqlThrottleMetricChan <- throttleMetrics
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
		log.Debugf("read instance key: %+v", key)

		probe := &mysql.Probe{
			Key:         *key,
			User:        clusterSettings.User,
			Password:    clusterSettings.Password,
			MetricQuery: clusterSettings.MetricQuery,
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
				log.Debugf("getting haproxy data from %s:%d", clusterSettings.HAProxySettings.Host, clusterSettings.HAProxySettings.Port)
				csv, err := haproxy.Read(clusterSettings.HAProxySettings.Host, clusterSettings.HAProxySettings.Port)
				if err != nil {
					return log.Errorf("Unable to get HAproxy data from %s:%d: %+v", clusterSettings.HAProxySettings.Host, clusterSettings.HAProxySettings.Port, err)
				}
				hosts, err := haproxy.ParseCsvHosts(csv, clusterSettings.HAProxySettings.PoolName)
				if err != nil {
					return log.Errorf("Unable to get HAproxy hosts from %s:%d: %+v", clusterSettings.HAProxySettings.Host, clusterSettings.HAProxySettings.Port, err)
				}
				log.Debugf("Read %+v hosts from haproxy %s:%d/%s", len(hosts), clusterSettings.HAProxySettings.Host, clusterSettings.HAProxySettings.Port, clusterSettings.HAProxySettings.PoolName)
				clusterProbes := &mysql.ClusterProbes{
					ClusterName: clusterName,
					Probes:      mysql.NewProbes(),
				}
				for _, host := range hosts {
					key := mysql.InstanceKey{Hostname: host, Port: clusterSettings.Port}
					addInstanceKey(&key, clusterSettings, clusterProbes.Probes)
				}
				throttler.mysqlClusterProbesChan <- clusterProbes
				return nil
			}
			if !clusterSettings.StaticHostsSettings.IsEmpty() {
				clusterProbes := &mysql.ClusterProbes{
					ClusterName: clusterName,
					Probes:      mysql.NewProbes(),
				}
				for _, host := range clusterSettings.StaticHostsSettings.Hosts {
					key, err := mysql.ParseInstanceKey(host, clusterSettings.Port)
					if err != nil {
						return log.Errore(err)
					}
					addInstanceKey(key, clusterSettings, clusterProbes.Probes)
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
	throttler.mysqlInventory.ClustersProbes[clusterProbes.ClusterName] = clusterProbes.Probes
	return nil
}

// synchronous aggregation of collected data
func (throttler *Throttler) aggregateMySQLMetrics() error {
	if !throttler.isLeader {
		return nil
	}
	for clusterName, probes := range throttler.mysqlInventory.ClustersProbes {
		metricName := fmt.Sprintf("mysql/%s", clusterName)
		aggregatedMetric := func() (worstMetric base.MetricResult) {
			var worstMetricValue float64

			// probes is known not to change. It can be *replaced*, but not changed.
			// so it's safe to iterate it
			if len(*probes) == 0 {
				return base.NoHostsMetricResult
			}
			for _, probe := range *probes {
				instanceMetricResult, ok := throttler.mysqlInventory.InstanceKeyMetrics[probe.Key]
				if !ok {
					return base.NoMetricResultYet
				}

				value, err := instanceMetricResult.Get()
				if err != nil {
					return instanceMetricResult
				}
				if value >= worstMetricValue {
					worstMetricValue = value
					worstMetric = instanceMetricResult
				}
			}
			return worstMetric
		}()
		go throttler.aggregatedMetrics.Set(metricName, aggregatedMetric, cache.DefaultExpiration)
	}
	return nil
}

func (throttler *Throttler) pushStatusToExpVar() {
	apps := []string{}
	throttledAppsExpVar.Do(func(appThrottlerStatus expvar.KeyValue) {
		apps = append(apps, appThrottlerStatus.Key)
	})

	for _, appName := range apps {
		throttled := new(expvar.Int)
		throttled.Set(0)
		throttledAppsExpVar.Set(appName, throttled)
	}

	for appName := range throttler.ThrottledAppsSnapshot() {
		throttled := throttledAppsExpVar.Get(appName)
		if throttled != nil {
			throttled.(*expvar.Int).Set(1)
		} else {
			throttled = new(expvar.Int)
			throttled.(*expvar.Int).Set(1)
			throttledAppsExpVar.Set(appName, throttled)
		}
	}
}

func (throttler *Throttler) GetMySQLClusterMetrics(clusterName string) (metricResult base.MetricResult, threshold float64) {
	if thresholdVal, found := throttler.mysqlClusterThresholds.Get(clusterName); found {
		threshold, _ = thresholdVal.(float64)
	} else {
		return base.NoSuchMetric, 0
	}

	metricName := fmt.Sprintf("mysql/%s", clusterName)
	if metricResultVal, found := throttler.aggregatedMetrics.Get(metricName); found {
		metricResult = metricResultVal.(base.MetricResult)
	} else {
		return base.NoSuchMetric, 0
	}
	return metricResult, threshold
}

func (throttler *Throttler) AggregatedMetrics() map[string]base.MetricResult {
	snapshot := make(map[string]base.MetricResult)
	for key, value := range throttler.aggregatedMetrics.Items() {
		metricResult, _ := value.Object.(base.MetricResult)
		snapshot[key] = metricResult
	}
	return snapshot
}

func (throttler *Throttler) ThrottleApp(appName string) {
	throttler.throttledApps.Set(appName, true, cache.DefaultExpiration)
}

func (throttler *Throttler) UnthrottleApp(appName string) {
	throttler.throttledApps.Delete(appName)
}

func (throttler *Throttler) IsAppThrottled(appName string) bool {
	_, found := throttler.throttledApps.Get(appName)
	return found
}

func (throttler *Throttler) AppRequestMetricResult(appName string, metricResultFunc base.MetricResultFunc) (metricResult base.MetricResult, threshold float64) {
	if throttler.IsAppThrottled(appName) {
		return base.AppDeniedMetric, 0
	}
	return metricResultFunc()
}
