package throttle

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/mysql"
	"github.com/patrickmn/go-cache"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
)

func TestExplainMySQLCluster(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() { config.Settings().Stores.MySQL = originalSettings }()
	recoveryThreshold := 100.0
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		IgnoreDialTcpErrors: true,
		FallbackCluster:     "explain-source",
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"explain-source": {
				ThrottleThreshold:      1,
				RequiredClusters:       []string{"explain-required"},
				ShadowRequiredClusters: []string{"explain-candidate", "explain-missing"},
			},
			"explain-required": {
				ThrottleThreshold:      10,
				RecoveryDurationMillis: 30000,
				RequiredClusters:       []string{"explain-shared"},
			},
			"explain-candidate": {
				ThrottleThreshold:      175,
				RecoveryThreshold:      &recoveryThreshold,
				RecoveryDurationMillis: 30000,
				CacheMillis:            5000,
				IgnoreHostsCount:       1,
				IgnoreHostsThreshold:   15,
				FailOnNoHosts:          true,
				RequiredClusters:       []string{"explain-shared", "explain-no-hosts"},
			},
			"explain-shared":   {ThrottleThreshold: 20},
			"explain-missing":  {ThrottleThreshold: 30},
			"explain-no-hosts": {ThrottleThreshold: 40, FailOnNoHosts: true},
		},
	}
	throttler := NewThrottler()
	setTestMySQLClusterMetric(throttler, "explain-source", 0.1, 1)
	setTestMySQLClusterMetric(throttler, "explain-candidate", 200, 175)
	throttler.aggregatedMetrics.SetDefault("mysql/explain-required", base.NewErrorMetricResult(3, fmt.Errorf("private-password: %w", base.RecoveryNotCompleteError)))
	throttler.aggregatedMetrics.SetDefault("mysql/explain-shared", base.NewErrorMetricResult(0, errors.New("query failed with private-password")))
	throttler.aggregatedMetrics.SetDefault("mysql/explain-no-hosts", base.NoHostsMetricResult)

	counterName := "shadow.http.mysql.explain-source.total"
	metrics.DefaultRegistry.Unregister(counterName)
	defer metrics.DefaultRegistry.Unregister(counterName)
	check := NewThrottlerCheck(throttler)
	result, err := check.ExplainMySQLCluster("explain-source")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "mysql/explain-source", result.MetricName)
	assert.Len(t, result.Metrics, 6)
	assert.Equal(t, "mysql/explain-source", result.Metrics[0].MetricName)
	byName := make(map[string]MySQLMetricExplanation)
	for _, metric := range result.Metrics {
		byName[metric.MetricName] = metric
	}
	source := byName["mysql/explain-source"]
	assert.Equal(t, "source", source.Mode)
	assert.Equal(t, http.StatusOK, source.StatusCode)
	assert.Equal(t, 0.1, *source.Value)
	assert.Equal(t, 1.0, source.RecoveryThreshold)

	candidate := byName["mysql/explain-candidate"]
	assert.Equal(t, "shadow", candidate.Mode)
	assert.Equal(t, http.StatusTooManyRequests, candidate.StatusCode)
	assert.Equal(t, 200.0, *candidate.Value)
	assert.Equal(t, 175.0, candidate.Threshold)
	assert.Equal(t, 100.0, candidate.RecoveryThreshold)
	assert.Equal(t, 30000, candidate.RecoveryDurationMillis)
	assert.Equal(t, 5000, candidate.CacheMillis)
	assert.True(t, candidate.FailOnNoHosts)
	assert.Equal(t, 1, candidate.IgnoreHostsCount)
	assert.Equal(t, 15.0, candidate.IgnoreHostsThreshold)
	assert.True(t, candidate.IgnoreDialTcpErrors)
	assert.Equal(t, []string{"explain-shared", "explain-no-hosts"}, candidate.RequiredClusters)

	required := byName["mysql/explain-required"]
	assert.Equal(t, "enforced", required.Mode)
	assert.Equal(t, http.StatusTooManyRequests, required.StatusCode)
	assert.Equal(t, 3.0, *required.Value)
	assert.Equal(t, base.RecoveryNotCompleteError.Error(), required.Message)
	shared := byName["mysql/explain-shared"]
	assert.Equal(t, "enforced", shared.Mode)
	assert.Equal(t, http.StatusInternalServerError, shared.StatusCode)
	assert.Nil(t, shared.Value)
	assert.Equal(t, "Probe error; consult server logs", shared.Message)
	assert.Equal(t, "shadow", byName["mysql/explain-no-hosts"].Mode)
	assert.Equal(t, base.NoHostsError.Error(), byName["mysql/explain-no-hosts"].Message)
	assert.Equal(t, http.StatusInternalServerError, byName["mysql/explain-no-hosts"].StatusCode)
	assert.Nil(t, byName["mysql/explain-missing"].Value)
	assert.Equal(t, http.StatusInternalServerError, byName["mysql/explain-missing"].StatusCode)
	assert.Nil(t, metrics.DefaultRegistry.Get(counterName))
	assert.Empty(t, throttler.mysqlRecoveryState)
	_, err = check.ExplainMySQLCluster("unknown")
	assert.Equal(t, base.NoSuchMetricError, err)
}

func TestExplainMySQLClusterPromotesSharedShadowDependency(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() { config.Settings().Stores.MySQL = originalSettings }()
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"root": {RequiredClusters: []string{"first", "second"}},
			"first": {
				ShadowRequiredClusters: []string{"shared"},
			},
			"second": {RequiredClusters: []string{"shared"}},
			"shared": {RequiredClusters: []string{"child"}},
			"child":  {},
		},
	}
	result, err := NewThrottlerCheck(NewThrottler()).ExplainMySQLCluster("root")
	if !assert.NoError(t, err) {
		return
	}
	for _, metric := range result.Metrics {
		if metric.MetricName == "mysql/shared" || metric.MetricName == "mysql/child" {
			assert.Equal(t, "enforced", metric.Mode)
		}
	}
}

func TestExplainMySQLClusterInvalidValues(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() { config.Settings().Stores.MySQL = originalSettings }()
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{"root": {ThrottleThreshold: 1}},
	}
	for _, value := range []float64{math.NaN(), math.Inf(1)} {
		throttler := NewThrottler()
		setTestMySQLClusterMetric(throttler, "root", value, 1)
		result, err := NewThrottlerCheck(throttler).ExplainMySQLCluster("root")
		if !assert.NoError(t, err) {
			continue
		}
		assert.Equal(t, http.StatusInternalServerError, result.Metrics[0].StatusCode)
		assert.Nil(t, result.Metrics[0].Value)
		assert.Equal(t, "invalid metric value", result.Metrics[0].Message)
	}
}

func TestRefreshMySQLInventoryStaticHostsIgnoreSettings(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	tests := []struct {
		name                 string
		ignoreHostsCount     int
		ignoreHostsThreshold float64
		highMetric           float64
		want                 float64
	}{
		{name: "default keeps worst host", highMetric: 21.5, want: 21.5},
		{name: "ignores host above threshold", ignoreHostsCount: 1, ignoreHostsThreshold: 15, highMetric: 21.5, want: 0.05},
		{name: "keeps host below threshold", ignoreHostsCount: 1, ignoreHostsThreshold: 15, highMetric: 14.9, want: 14.9},
		{name: "keeps host at threshold", ignoreHostsCount: 1, ignoreHostsThreshold: 15, highMetric: 15, want: 15},
		{name: "threshold alone ignores no hosts", ignoreHostsThreshold: 15, highMetric: 21.5, want: 21.5},
		{name: "zero threshold is unconditional", ignoreHostsCount: 1, highMetric: 21.5, want: 0.05},
		{name: "retains at least one host", ignoreHostsCount: 2, highMetric: 21.5, want: 0.05},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const clusterName = "static"
			config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
				Clusters: map[string]*config.MySQLClusterConfigurationSettings{
					clusterName: {
						Port:                 config.DefaultMySQLPort,
						IgnoreHostsCount:     test.ignoreHostsCount,
						IgnoreHostsThreshold: test.ignoreHostsThreshold,
						StaticHostsSettings: config.StaticHostsConfigurationSettings{
							Hosts: []string{key1.Hostname, key2.StringCode()},
						},
					},
				},
			}
			throttler := NewThrottler()
			throttler.isLeader = true
			throttler.mysqlClusterProbesChan = make(chan *mysql.ClusterProbes, 1)
			assert.NoError(t, throttler.refreshMySQLInventory())

			var probes *mysql.ClusterProbes
			select {
			case probes = <-throttler.mysqlClusterProbesChan:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for static-host probes")
			}
			if probes == nil || probes.InstanceProbes == nil {
				t.Fatal("missing static-host probes")
			}
			assert.Equal(t, clusterName, probes.ClusterName)
			assert.Equal(t, test.ignoreHostsCount, probes.IgnoreHostsCount)
			assert.Equal(t, test.ignoreHostsThreshold, probes.IgnoreHostsThreshold)
			if len(*probes.InstanceProbes) != 2 {
				t.Fatalf("probe count = %d, want 2", len(*probes.InstanceProbes))
			}
			for _, key := range []mysql.InstanceKey{key1, key2} {
				probe := (*probes.InstanceProbes)[key]
				want := mysql.Probe{Key: key}
				if probe == nil || *probe != want {
					t.Fatalf("probe for %s = %+v, want %+v", key.StringCode(), probe, want)
				}
			}
			assert.NoError(t, throttler.updateMySQLClusterProbes(probes))

			inventory := throttler.mysqlInventory
			inventory.InstanceKeyMetrics[mysql.GetClusterInstanceKey(clusterName, &key1)] = base.NewSimpleMetricResult(test.highMetric)
			inventory.InstanceKeyMetrics[mysql.GetClusterInstanceKey(clusterName, &key2)] = base.NewSimpleMetricResult(0.05)
			metric := aggregateMySQLProbes(
				inventory.ClustersProbes[clusterName], clusterName,
				inventory.InstanceKeyMetrics, inventory.ClusterInstanceHttpChecks,
				inventory.IgnoreHostsCount[clusterName], false, inventory.IgnoreHostsThreshold[clusterName],
			)
			value, err := metric.Get()
			assert.NoError(t, err)
			assert.Equal(t, test.want, value)
		})
	}
}

func Test_checkAppMetricResult(t *testing.T) {
	throttler := NewThrottler()

	t.Run("threshold exceeded", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewSimpleMetricResult(10.0), 1.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.Equal(t, 10.0, result.Value)
		assert.EqualError(t, result.Error, base.ThresholdExceededError.Error())
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	})
	t.Run("not throttled", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewSimpleMetricResult(1.0), 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.Equal(t, 1.0, result.Value)
		assert.Nil(t, result.Error)
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})
	t.Run("no hosts", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NoHostsMetricResult, 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.Equal(t, 0.0, result.Value)
		assert.EqualError(t, result.Error, base.NoHostsError.Error())
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})
	t.Run("no hosts fail closed", func(t *testing.T) {
		originalSettings := config.Settings().Stores.MySQL
		defer func() {
			config.Settings().Stores.MySQL = originalSettings
		}()
		config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
			Clusters: map[string]*config.MySQLClusterConfigurationSettings{
				"test-cluster": {FailOnNoHosts: true},
			},
		}

		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NoHostsMetricResult, 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.EqualError(t, result.Error, base.NoHostsError.Error())
		assert.Equal(t, http.StatusInternalServerError, result.StatusCode)
	})
	t.Run("recovery period throttles", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewErrorMetricResult(5.0, base.RecoveryNotCompleteError), 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.EqualError(t, result.Error, base.RecoveryNotCompleteError.Error())
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	})
	t.Run("low priority denied", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		throttler.nonLowPriorityAppRequestsThrottled.SetDefault("mysql/test-cluster", true)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewSimpleMetricResult(1.0), 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{LowPriority: true})
		assert.EqualError(t, result.Error, base.AppDeniedError.Error())
		assert.Equal(t, http.StatusExpectationFailed, result.StatusCode)

	})
}

func setTestMySQLClusterMetric(throttler *Throttler, clusterName string, value, threshold float64) {
	throttler.mysqlClusterThresholds.SetDefault(clusterName, threshold)
	throttler.aggregatedMetrics.SetDefault("mysql/"+clusterName, base.NewSimpleMetricResult(value))
}

func SetTestMySQLClusterMetric(throttler *Throttler, clusterName string, value, threshold float64) {
	setTestMySQLClusterMetric(throttler, clusterName, value, threshold)
}

func newTestMySQLFallbackCheck(fallbackCluster string) (*ThrottlerCheck, *Throttler) {
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		FallbackCluster: fallbackCluster,
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"fallback": {},
			"exact":    {},
		},
	}

	throttler := NewThrottler()
	setTestMySQLClusterMetric(throttler, "fallback", 1.0, 10.0)
	return NewThrottlerCheck(throttler), throttler
}

func TestCheckMySQLFallback(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	t.Run("exact name wins", func(t *testing.T) {
		check, throttler := newTestMySQLFallbackCheck("fallback")
		setTestMySQLClusterMetric(throttler, "exact", 2.0, 20.0)
		result := check.Check("test-app", "mysql", "exact", "", &CheckFlags{})
		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.Equal(t, 2.0, result.Value)
		assert.Equal(t, 20.0, result.Threshold)
	})

	t.Run("unknown name falls back", func(t *testing.T) {
		check, _ := newTestMySQLFallbackCheck("fallback")
		result := check.Check("test-app", "mysql", "unknown", "", &CheckFlags{})
		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.Equal(t, 1.0, result.Value)
		assert.Equal(t, 10.0, result.Threshold)
	})

	t.Run("exact name with missing runtime metric does not fall back", func(t *testing.T) {
		check, throttler := newTestMySQLFallbackCheck("fallback")
		throttler.mysqlClusterThresholds.SetDefault("exact", 20.0)
		result := check.Check("test-app", "mysql", "exact", "", &CheckFlags{})
		assert.Equal(t, http.StatusNotFound, result.StatusCode)
		assert.Equal(t, 20.0, result.Threshold)
	})

	t.Run("strict metric lookup does not fall back", func(t *testing.T) {
		_, throttler := newTestMySQLFallbackCheck("fallback")
		metric, threshold := throttler.getMySQLClusterMetrics("unknown")
		_, err := metric.Get()
		assert.Equal(t, base.NoSuchMetricError, err)
		assert.Equal(t, 0.0, threshold)
	})

	t.Run("no fallback remains not found", func(t *testing.T) {
		check, throttler := newTestMySQLFallbackCheck("")
		setTestMySQLClusterMetric(throttler, "unknown", 1.0, 10.0)
		result := check.Check("test-app", "mysql", "unknown", "", &CheckFlags{})
		assert.Equal(t, http.StatusNotFound, result.StatusCode)
	})

	t.Run("unsupported store remains not found", func(t *testing.T) {
		check, _ := newTestMySQLFallbackCheck("fallback")
		result := check.Check("test-app", "redis", "unknown", "", &CheckFlags{})
		assert.Equal(t, http.StatusNotFound, result.StatusCode)
	})
}

func TestCheckMySQLFallbackUsesCanonicalState(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	tests := []struct {
		name    string
		flags   *CheckFlags
		prepare func(*Throttler)
		want    int
	}{
		{
			name:  "per-store app throttle",
			flags: &CheckFlags{},
			prepare: func(throttler *Throttler) {
				throttler.ThrottleApp("test-app/fallback", time.Time{}, 1)
			},
			want: http.StatusExpectationFailed,
		},
		{
			name:  "low priority state",
			flags: &CheckFlags{LowPriority: true},
			prepare: func(throttler *Throttler) {
				throttler.nonLowPriorityAppRequestsThrottled.SetDefault("mysql/fallback", true)
			},
			want: http.StatusExpectationFailed,
		},
		{
			name:  "share domain health",
			flags: &CheckFlags{},
			prepare: func(throttler *Throttler) {
				throttler.shareDomainMetricHealth.SetDefault("mysql/fallback", &base.MetricHealth{SecondsSinceLastHealthy: 1})
			},
			want: http.StatusTooManyRequests,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check, throttler := newTestMySQLFallbackCheck("fallback")
			test.prepare(throttler)
			result := check.Check("test-app", "mysql", "unknown", "", test.flags)
			assert.Equal(t, test.want, result.StatusCode)
		})
	}
}

func TestCheckMySQLRequiredClusters(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas": {
				RequiredClusters: []string{"primary", "proxysql"},
			},
			"primary":  {},
			"proxysql": {},
		},
	}

	newCheck := func() (*ThrottlerCheck, *Throttler) {
		throttler := NewThrottler()
		setTestMySQLClusterMetric(throttler, "replicas", 1.0, 5.0)
		setTestMySQLClusterMetric(throttler, "primary", 10.0, 50.0)
		setTestMySQLClusterMetric(throttler, "proxysql", 20.0, 80.0)
		return NewThrottlerCheck(throttler), throttler
	}

	t.Run("all required clusters healthy", func(t *testing.T) {
		check, _ := newCheck()
		result := check.Check("transitions", "mysql", "replicas", "", &CheckFlags{})
		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.Equal(t, 1.0, result.Value)
		assert.Equal(t, 5.0, result.Threshold)
	})

	t.Run("primary cluster blocks admission", func(t *testing.T) {
		check, throttler := newCheck()
		setTestMySQLClusterMetric(throttler, "primary", 60.0, 50.0)
		result := check.Check("transitions", "mysql", "replicas", "", &CheckFlags{})
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
		assert.Equal(t, 60.0, result.Value)
		assert.Equal(t, 50.0, result.Threshold)
		assert.Equal(t, "mysql/primary", result.MetricName)
	})

	t.Run("ProxySQL cluster blocks admission", func(t *testing.T) {
		check, throttler := newCheck()
		setTestMySQLClusterMetric(throttler, "proxysql", 90.0, 80.0)
		result := check.Check("transitions", "mysql", "replicas", "", &CheckFlags{})
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
		assert.Equal(t, 90.0, result.Value)
		assert.Equal(t, 80.0, result.Threshold)
	})

	t.Run("read threshold override only applies to requested cluster", func(t *testing.T) {
		check, throttler := newCheck()
		setTestMySQLClusterMetric(throttler, "primary", 60.0, 50.0)
		result := check.Check("transitions", "mysql", "replicas", "", &CheckFlags{
			ReadCheck:         true,
			OverrideThreshold: 100.0,
		})
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
		assert.Equal(t, 50.0, result.Threshold)
	})

	t.Run("read threshold override bypasses only the requested cluster recovery latch", func(t *testing.T) {
		check, throttler := newCheck()
		throttler.aggregatedMetrics.Set(
			"mysql/replicas",
			base.NewErrorMetricResult(45.0, base.RecoveryNotCompleteError),
			cache.DefaultExpiration,
		)
		result := check.Check("transitions", "mysql", "replicas", "", &CheckFlags{
			ReadCheck:         true,
			OverrideThreshold: 100.0,
		})
		assert.Equal(t, http.StatusOK, result.StatusCode)

		throttler.aggregatedMetrics.Set(
			"mysql/primary",
			base.NewErrorMetricResult(40.0, base.RecoveryNotCompleteError),
			cache.DefaultExpiration,
		)
		result = check.Check("transitions", "mysql", "replicas", "", &CheckFlags{
			ReadCheck:         true,
			OverrideThreshold: 100.0,
		})
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
		assert.Equal(t, "mysql/primary", result.MetricName)
	})

	t.Run("missing required metric fails closed", func(t *testing.T) {
		check, throttler := newCheck()
		throttler.aggregatedMetrics.Delete("mysql/primary")
		result := check.Check("transitions", "mysql", "replicas", "", &CheckFlags{OKIfNotExists: true})
		assert.Equal(t, http.StatusInternalServerError, result.StatusCode)
		assert.EqualError(t, result.Error, `required MySQL cluster metric "primary" is unavailable`)
		assert.Equal(t, "mysql/primary", result.MetricName)
	})
}

func TestStabilizeMySQLMetric(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	recoveryThreshold := 40.0
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"primary": {
				ThrottleThreshold:      50.0,
				RecoveryThreshold:      &recoveryThreshold,
				RecoveryDurationMillis: 5000,
			},
		},
	}

	throttler := NewThrottler()
	start := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)

	assertMetric := func(t *testing.T, result base.MetricResult, wantValue float64, wantErr error) {
		t.Helper()
		value, err := result.Get()
		assert.Equal(t, wantValue, value)
		assert.Equal(t, wantErr, err)
	}

	assertMetric(t, throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(60), start), 60, nil)
	assertMetric(t, throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(45), start.Add(time.Second)), 45, base.RecoveryNotCompleteError)
	assertMetric(t, throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(35), start.Add(2*time.Second)), 35, base.RecoveryNotCompleteError)
	assertMetric(t, throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(35), start.Add(6*time.Second)), 35, base.RecoveryNotCompleteError)
	assertMetric(t, throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(35), start.Add(7*time.Second)), 35, nil)
}

func TestStabilizeMySQLMetricResetsRecoveryWindow(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"primary": {
				ThrottleThreshold:      50.0,
				RecoveryDurationMillis: 5000,
			},
		},
	}

	throttler := NewThrottler()
	start := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)

	throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(60), start)
	throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(40), start.Add(time.Second))
	throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(55), start.Add(3*time.Second))
	result := throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(40), start.Add(7*time.Second))
	_, err := result.Get()
	assert.Equal(t, base.RecoveryNotCompleteError, err)
}

func TestStabilizeMySQLMetricRequiresHealthyWindowAfterStartup(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"primary": {
				ThrottleThreshold:      50.0,
				RecoveryDurationMillis: 5000,
			},
		},
	}

	throttler := NewThrottler()
	start := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)

	result := throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(30), start)
	_, err := result.Get()
	assert.Equal(t, base.RecoveryNotCompleteError, err)

	result = throttler.stabilizeMySQLMetric("primary", base.NewSimpleMetricResult(30), start.Add(5*time.Second))
	_, err = result.Get()
	assert.Nil(t, err)
	assert.Equal(t, int64(0), metrics.Get("recovery.mysql.primary.active").(metrics.Gauge).Value())
}

func TestUpdateLeadershipResetsRecoveryStateOnRegain(t *testing.T) {
	throttler := NewThrottler()
	throttler.isLeader = true
	throttler.mysqlRecoveryState["primary"] = &mysqlClusterRecoveryState{throttled: false}

	throttler.updateLeadership(false)
	assert.Len(t, throttler.mysqlRecoveryState, 1)

	throttler.updateLeadership(true)
	assert.Empty(t, throttler.mysqlRecoveryState)
}

func TestRequiredMySQLClustersHealthyForMemcache(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() {
		config.Settings().Stores.MySQL = originalSettings
	}()

	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas": {RequiredClusters: []string{"primary"}},
			"primary":  {ThrottleThreshold: 50.0},
		},
	}
	throttler := NewThrottler()
	aggregatedMetrics := map[string]base.MetricResult{
		"replicas": base.NewSimpleMetricResult(1.0),
		"primary":  base.NewSimpleMetricResult(40.0),
	}

	assert.True(t, throttler.requiredMySQLClustersHealthy("replicas", aggregatedMetrics, make(map[string]bool)))

	aggregatedMetrics["primary"] = base.NewSimpleMetricResult(60.0)
	assert.False(t, throttler.requiredMySQLClustersHealthy("replicas", aggregatedMetrics, make(map[string]bool)))

	delete(aggregatedMetrics, "primary")
	assert.False(t, throttler.requiredMySQLClustersHealthy("replicas", aggregatedMetrics, make(map[string]bool)))
}
