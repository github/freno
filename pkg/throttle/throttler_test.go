package throttle

import (
	"net/http"
	"testing"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/stretchr/testify/assert"
)

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
