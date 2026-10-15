package throttle

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/mysql"
	"github.com/patrickmn/go-cache"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
)

func resetShadowCounters(t *testing.T) {
	clear := func() {
		metrics.DefaultRegistry.Each(func(name string, _ interface{}) {
			if strings.HasPrefix(name, "shadow.") {
				metrics.DefaultRegistry.Unregister(name)
			}
		})
	}
	clear()
	t.Cleanup(clear)
}

func shadowCount(name string) int64 {
	if counter := metrics.Get(name); counter != nil {
		return counter.(metrics.Counter).Count()
	}
	return 0
}

func newShadowTestCheck(t *testing.T) (*ThrottlerCheck, *Throttler) {
	original := config.Settings().Stores.MySQL
	t.Cleanup(func() { config.Settings().Stores.MySQL = original })
	resetShadowCounters(t)
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		FallbackCluster: "root",
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"root":     {ThrottleThreshold: 5, RequiredClusters: []string{"required"}, ShadowRequiredClusters: []string{"shadow"}},
			"required": {ThrottleThreshold: 10},
			"shadow":   {ThrottleThreshold: 20, FailOnNoHosts: true},
		},
	}
	throttler := NewThrottler()
	setTestMySQLClusterMetric(throttler, "root", 1, 5)
	setTestMySQLClusterMetric(throttler, "required", 2, 10)
	setTestMySQLClusterMetric(throttler, "shadow", 3, 20)
	return NewThrottlerCheck(throttler), throttler
}

func TestShadowHTTPDoesNotChangeAdmission(t *testing.T) {
	for _, test := range []struct {
		name       string
		metric     base.MetricResult
		failOpen   bool
		wantStatus int
	}{
		{"healthy", base.NewSimpleMetricResult(20), false, 0},
		{"overloaded", base.NewSimpleMetricResult(21), false, 429},
		{"collection error", base.NewErrorMetricResult(0, errors.New("query denied")), false, 500},
		{"not collected", base.NoMetricResultYet, false, 500},
		{"missing metric", base.NoSuchMetric, false, 500},
		{"no hosts fail closed", base.NoHostsMetricResult, false, 500},
		{"no hosts legacy", base.NoHostsMetricResult, true, 0},
		{"recovering", base.NewErrorMetricResult(3, base.RecoveryNotCompleteError), false, 429},
	} {
		t.Run(test.name, func(t *testing.T) {
			check, throttler := newShadowTestCheck(t)
			config.Settings().Stores.MySQL.Clusters["shadow"].FailOnNoHosts = !test.failOpen
			throttler.aggregatedMetrics.Set("mysql/shadow", test.metric, cache.DefaultExpiration)
			before := check.checkMySQLCluster("test-app", "root", &CheckFlags{}, make(map[string]bool))
			result := check.Check("test-app", "mysql", "root", "", &CheckFlags{})
			assert.Equal(t, before, result)
			assert.Equal(t, http.StatusOK, result.StatusCode)
			assert.Equal(t, int64(1), shadowCount("shadow.http.mysql.root.total"))
			wantRejections := int64(0)
			if test.wantStatus != 0 {
				wantRejections = 1
				assert.Equal(t, int64(1), shadowCount(fmt.Sprintf("shadow.http.mysql.root.status.%d.would_reject", test.wantStatus)))
			}
			assert.Equal(t, wantRejections, shadowCount("shadow.http.mysql.root.would_reject"))
			assert.Equal(t, wantRejections, shadowCount("shadow.http.mysql.root.dependency.shadow.would_reject"))
			assert.Empty(t, throttler.nonLowPriorityAppRequestsThrottled.Items())
			assert.Empty(t, throttler.metricsHealth.Items())
			assert.Empty(t, throttler.mysqlRecoveryState)
			assert.Equal(t, int64(0), shadowCount("shadow.background.mysql.root.total"))
		})
	}
}

func TestShadowHTTPOnlyOtherwiseAllowedRequests(t *testing.T) {
	for _, rejected := range []string{"root", "required"} {
		t.Run(rejected, func(t *testing.T) {
			check, throttler := newShadowTestCheck(t)
			throttler.aggregatedMetrics.Set("mysql/"+rejected, base.NewSimpleMetricResult(100), cache.DefaultExpiration)
			result := check.Check("test-app", "mysql", "root", "", &CheckFlags{})
			assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
			assert.Equal(t, "mysql/"+rejected, result.MetricName)
			assert.Nil(t, metrics.Get("shadow.http.mysql.root.total"))
		})
	}
	t.Run("real app denial", func(t *testing.T) {
		check, throttler := newShadowTestCheck(t)
		throttler.ThrottleApp("test-app", time.Time{}, 1)
		assert.Equal(t, http.StatusExpectationFailed, check.Check("test-app", "mysql", "root", "", &CheckFlags{}).StatusCode)
		assert.Nil(t, metrics.Get("shadow.http.mysql.root.total"))
	})
}

func TestShadowHTTPStrictThresholdsAndPromotion(t *testing.T) {
	check, throttler := newShadowTestCheck(t)
	throttler.aggregatedMetrics.Set("mysql/root", base.NewErrorMetricResult(6, base.RecoveryNotCompleteError), cache.DefaultExpiration)
	throttler.aggregatedMetrics.Set("mysql/shadow", base.NewErrorMetricResult(3, base.RecoveryNotCompleteError), cache.DefaultExpiration)
	flags := &CheckFlags{ReadCheck: true, OverrideThreshold: 100}
	result := check.Check("test-app", "mysql", "unknown-client-name", "", flags)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, 100.0, result.Threshold)
	assert.Equal(t, int64(1), shadowCount("shadow.http.mysql.root.status.429.would_reject"))
	assert.Nil(t, metrics.Get("shadow.http.mysql.unknown-client-name.total"))

	root := config.Settings().Stores.MySQL.Clusters["root"]
	root.RequiredClusters = append(root.RequiredClusters, root.ShadowRequiredClusters...)
	root.ShadowRequiredClusters = nil
	result = check.Check("test-app", "mysql", "root", "", flags)
	assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	assert.Equal(t, "mysql/shadow", result.MetricName)
	assert.Equal(t, 20.0, result.Threshold)
	assert.Equal(t, int64(1), shadowCount("shadow.http.mysql.root.total"))
}

func TestShadowSkipsAppPolicyAndSelfChecks(t *testing.T) {
	check, throttler := newShadowTestCheck(t)
	throttler.ThrottleApp("test-app/shadow", time.Time{}, 1)
	throttler.shareDomainMetricHealth.SetDefault("mysql/shadow", &base.MetricHealth{SecondsSinceLastHealthy: 10})
	throttler.nonLowPriorityAppRequestsThrottled.SetDefault("mysql/shadow", true)
	result := check.Check("test-app", "mysql", "root", "", &CheckFlags{LowPriority: true})
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, int64(0), shadowCount("shadow.http.mysql.root.would_reject"))

	throttler.aggregatedMetrics.Set("mysql/shadow", base.NewSimpleMetricResult(100), cache.DefaultExpiration)
	result = check.localCheck("mysql/root")
	deadline := time.Now().Add(time.Second)
	for {
		if _, finished := throttler.recentApps.Get(frenoShareDmainAppName + "/local"); finished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shared-domain self-check did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, int64(1), shadowCount("shadow.http.mysql.root.total"))
	assert.Equal(t, int64(0), shadowCount("shadow.http.mysql.root.would_reject"))
	_, markedHealthy := throttler.metricsHealth.Get("mysql/root")
	assert.True(t, markedHealthy)
}

func TestShadowGraphIncludesNestedDependenciesOnce(t *testing.T) {
	newShadowTestCheck(t)
	clusters := config.Settings().Stores.MySQL.Clusters
	clusters["root"].ShadowRequiredClusters = nil
	clusters["root"].RequiredClusters = append(clusters["root"].RequiredClusters, "shared")
	clusters["required"].ShadowRequiredClusters = []string{"shadow", "shadow"}
	clusters["shadow"].RequiredClusters = []string{"shared", "nested"}
	clusters["shadow"].ShadowRequiredClusters = []string{"extra"}
	clusters["shared"] = &config.MySQLClusterConfigurationSettings{}
	clusters["nested"] = &config.MySQLClusterConfigurationSettings{}
	clusters["extra"] = &config.MySQLClusterConfigurationSettings{RequiredClusters: []string{"nested"}}
	calls := make(map[string]int)
	evaluate := func(name string) *CheckResult {
		calls[name]++
		return NewCheckResult(http.StatusOK, 0, 1, nil)
	}
	observeMySQLShadowDependencies("http", "root", evaluate)
	assert.Equal(t, map[string]int{"shadow": 1, "nested": 1, "extra": 1}, calls)
	assert.Equal(t, int64(1), shadowCount("shadow.http.mysql.root.total"))
	assert.Equal(t, int64(0), shadowCount("shadow.http.mysql.root.would_reject"))

	calls = make(map[string]int)
	observeMySQLShadowDependencies("http", "root", func(name string) *CheckResult {
		if name == "nested" {
			return NewErrorCheckResult(http.StatusInternalServerError, errors.New("missing"))
		}
		return evaluate(name)
	})
	assert.Equal(t, int64(1), shadowCount("shadow.http.mysql.root.dependency.nested.would_reject"))
	assert.Equal(t, 0, calls["extra"])
}

func addShadowInventoryMetric(throttler *Throttler, name string, metric base.MetricResult) {
	key := mysql.InstanceKey{Hostname: name, Port: 3306}
	probes := mysql.Probes{key: &mysql.Probe{Key: key}}
	throttler.mysqlInventory.ClustersProbes[name] = &probes
	throttler.mysqlInventory.InstanceKeyMetrics[mysql.GetClusterInstanceKey(name, &key)] = metric
}

func TestShadowBackgroundSeparateFromHTTP(t *testing.T) {
	for _, test := range []struct {
		name       string
		root       base.MetricResult
		required   base.MetricResult
		shadow     base.MetricResult
		wantTotal  int64
		wantReject int64
	}{
		{"healthy", base.NewSimpleMetricResult(1), base.NewSimpleMetricResult(2), base.NewSimpleMetricResult(3), 1, 0},
		{"overload", base.NewSimpleMetricResult(1), base.NewSimpleMetricResult(2), base.NewSimpleMetricResult(21), 1, 1},
		{"legacy no hosts still unsafe for memcache", base.NewSimpleMetricResult(1), base.NewSimpleMetricResult(2), base.NoHostsMetricResult, 1, 1},
		{"root denied", base.NewSimpleMetricResult(6), base.NewSimpleMetricResult(2), base.NewSimpleMetricResult(21), 0, 0},
		{"required denied", base.NewSimpleMetricResult(1), base.NewSimpleMetricResult(11), base.NewSimpleMetricResult(21), 0, 0},
		{"root error", base.NoMetricResultYet, base.NewSimpleMetricResult(2), base.NewSimpleMetricResult(21), 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, throttler := newShadowTestCheck(t)
			config.Settings().Stores.MySQL.Clusters["shadow"].FailOnNoHosts = false
			addShadowInventoryMetric(throttler, "root", test.root)
			addShadowInventoryMetric(throttler, "required", test.required)
			addShadowInventoryMetric(throttler, "shadow", test.shadow)
			assert.NoError(t, throttler.aggregateMySQLMetrics())
			assert.Nil(t, metrics.Get("shadow.background.mysql.root.total"))
			throttler.isLeader = true
			assert.NoError(t, throttler.aggregateMySQLMetrics())
			assert.Equal(t, test.wantTotal, shadowCount("shadow.background.mysql.root.total"))
			assert.Equal(t, test.wantReject, shadowCount("shadow.background.mysql.root.would_reject"))
			assert.Nil(t, metrics.Get("shadow.http.mysql.root.total"))
		})
	}
}

// Read one real publication command, rather than inferring memcache behavior from
// the dependency predicate alone.
func TestShadowMemcachePublicationAndPromotion(t *testing.T) {
	for _, enforce := range []bool{false, true} {
		t.Run(fmt.Sprintf("enforced=%t", enforce), func(t *testing.T) {
			_, throttler := newShadowTestCheck(t)
			root := config.Settings().Stores.MySQL.Clusters["root"]
			root.RequiredClusters = nil
			if enforce {
				root.RequiredClusters = root.ShadowRequiredClusters
				root.ShadowRequiredClusters = nil
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			type publication struct {
				command string
				err     error
			}
			published := make(chan publication, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					published <- publication{err: err}
					return
				}
				defer conn.Close()
				if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
					published <- publication{err: err}
					return
				}
				reader := bufio.NewReader(conn)
				line, err := reader.ReadString('\n')
				if err == nil && strings.HasPrefix(line, "set ") {
					_, err = reader.ReadString('\n')
				}
				if err == nil {
					response := "STORED\r\n"
					if strings.HasPrefix(line, "delete ") {
						response = "DELETED\r\n"
					}
					_, err = io.WriteString(conn, response)
				}
				published <- publication{command: line, err: err}
			}()
			throttler.memcacheClient = memcache.New(listener.Addr().String())
			throttler.memcachePath = "shadow-test"
			throttler.isLeader = true
			addShadowInventoryMetric(throttler, "root", base.NewSimpleMetricResult(1))
			assert.NoError(t, throttler.aggregateMySQLMetrics())
			select {
			case result := <-published:
				assert.NoError(t, result.err)
				want := "set shadow-test/mysql/root "
				if enforce {
					want = "delete shadow-test/mysql/root"
				}
				assert.True(t, strings.HasPrefix(result.command, want), result.command)
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for memcache publication")
			}
			if !enforce {
				assert.Equal(t, int64(1), shadowCount("shadow.background.mysql.root.status.500.would_reject"))
			}
		})
	}
}
