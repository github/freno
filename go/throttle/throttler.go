package throttle

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/config"
	"github.com/github/freno/go/group"
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

type Throttler struct {
	isLeader bool

	mysqlThrottleMetricChan chan *mysql.MySQLThrottleMetric
	mysqlInventoryChan      chan *mysql.MySQLInventory
	mysqlClusterProbesChan  chan *mysql.ClusterConnectionProbes

	mysqlInventory *mysql.MySQLInventory

	aggregatedMetrics *cache.Cache
}

func NewThrottler() *Throttler {
	throttler := &Throttler{
		isLeader: false,

		mysqlThrottleMetricChan: make(chan *mysql.MySQLThrottleMetric),

		mysqlInventoryChan:     make(chan *mysql.MySQLInventory, 1),
		mysqlClusterProbesChan: make(chan *mysql.ClusterConnectionProbes),
		mysqlInventory:         mysql.NewMySQLInventory(),

		aggregatedMetrics: cache.New(aggregatedMetricsExpiration, aggregatedMetricsCleanup),
	}
	return throttler
}

func (throttler *Throttler) Operate() {
	leaderCheckTick := time.Tick(leaderCheckInterval)
	mysqlCollectTick := time.Tick(mysqlCollectInterval)
	mysqlRefreshTick := time.Tick(mysqlRefreshInterval)
	mysqlAggregateTick := time.Tick(mysqlAggreateInterval)
	for {
		select {
		case <-leaderCheckTick:
			{
				// sparsse
				throttler.isLeader = group.IsLeader()
			}
		case <-mysqlCollectTick:
			{
				// frequent
				throttler.collectMySQLMetrics()
			}
		case metric := <-throttler.mysqlThrottleMetricChan:
			{
				// incoming MySQL metric, frequent, as result of collectMySQLMetrics()
				log.Debugf("got metrics for %+v", metric)
				throttler.mysqlInventory.InstanceKeyMetrics[metric.Key] = metric
			}
		case <-mysqlRefreshTick:
			{
				// sparse
				go throttler.refreshMySQLInventory()
			}
		case connectionProbes := <-throttler.mysqlClusterProbesChan:
			{
				// incoming structural update, sparse, as result of refreshMySQLInventory()
				throttler.onUpdatedMySQLClusterProbes(connectionProbes)
			}
		case <-mysqlAggregateTick:
			{
				throttler.aggregateMySQLMetrics()
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
	for _, connectionProbes := range throttler.mysqlInventory.ClustersProbes {
		connectionProbes := connectionProbes
		go func() {
			// connectionProbes is known not to change. It can be *replaced*, but not changed.
			// so it's safe to iterate it
			for _, connectionProbe := range *connectionProbes {
				connectionProbe := connectionProbe
				go func() {
					// Avoid querying the same server twice at the same time. If previous read is still there,
					// we avoid re-reading it.
					if !atomic.CompareAndSwapInt64(&connectionProbe.InProgress, 0, 1) {
						return
					}
					defer atomic.StoreInt64(&connectionProbe.InProgress, 0)
					throttleMetrics := mysql.ReadThrottleMetric(connectionProbe)
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

	for clusterName, clusterSettings := range config.Settings().Stores.MySQL.Clusters {
		clusterName := clusterName
		clusterSettings := clusterSettings
		// config may dynamically change, but internal structure (config.Settings().Stores.MySQL.Clusters in our case)
		// is immutable and can only be _replaced_. Hence, it's safe to read in a goroutine:
		go func() error {
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
				clusterConnectionProbes := &mysql.ClusterConnectionProbes{
					ClusterName: clusterName,
					Probes:      mysql.NewConnectionProbes(),
				}
				for _, host := range hosts {
					key := mysql.InstanceKey{Hostname: host, Port: clusterSettings.Port}
					log.Debugf("read instance key: %+v", key)

					connectionProbe := mysql.NewConnectionProbe()
					connectionProbe.Key = key
					connectionProbe.User = clusterSettings.User
					connectionProbe.Password = clusterSettings.Password
					connectionProbe.MetricQuery = clusterSettings.MetricQuery
					(*clusterConnectionProbes.Probes)[key] = connectionProbe
				}
				throttler.mysqlClusterProbesChan <- clusterConnectionProbes
			}
			return nil
		}()
	}
	return nil
}

// synchronous update of inventory
func (throttler *Throttler) onUpdatedMySQLClusterProbes(clusterProbes *mysql.ClusterConnectionProbes) error {
	log.Debugf("onMySQLClusterConnectionProbes: %s", clusterProbes.ClusterName)
	throttler.mysqlInventory.ClustersProbes[clusterProbes.ClusterName] = clusterProbes.Probes
	return nil
}

// synchronous aggregation of collected data
func (throttler *Throttler) aggregateMySQLMetrics() error {
	if !throttler.isLeader {
		return nil
	}
	for clusterName, connectionProbes := range throttler.mysqlInventory.ClustersProbes {
		metricName := fmt.Sprintf("mysql/%s", clusterName)
		aggregatedMetric := func() (worstMetric base.MetricResult) {
			var worstMetricValue float64

			// connectionProbes is known not to change. It can be *replaced*, but not changed.
			// so it's safe to iterate it
			if len(*connectionProbes) == 0 {
				return base.NoHostsMetricResult
			}
			for _, connectionProbe := range *connectionProbes {
				instanceMetricResult, ok := throttler.mysqlInventory.InstanceKeyMetrics[connectionProbe.Key]
				if !ok {
					return base.NoMetricResultYet
				}

				log.Debugf(">>> metric is %+v", instanceMetricResult)
				value, err := instanceMetricResult.Get()
				if err != nil {
					return instanceMetricResult
				}
				if value >= worstMetricValue {
					worstMetric = instanceMetricResult
				}
			}
			return worstMetric
		}()
		val, err := aggregatedMetric.Get()
		log.Debugf("###>>> aggregated metric %+v, %+v", val, err)
		throttler.aggregatedMetrics.Set(metricName, aggregatedMetric, cache.DefaultExpiration)
	}
	return nil
}
