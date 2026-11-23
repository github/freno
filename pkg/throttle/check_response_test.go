package throttle

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCheckKeepsRequestedMeasurementAndReportsDependencies(t *testing.T) {
	original := config.Settings().Stores.MySQL
	t.Cleanup(func() { config.Settings().Stores.MySQL = original })
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas": {RequiredClusters: []string{"primary", "other"}},
			"primary":  {FailOnNoHosts: true},
			"other":    {},
		},
	}
	for _, test := range []struct {
		name        string
		replica     base.MetricResult
		primary     base.MetricResult
		shared      bool
		wantStatus  int
		wantBlocker string
		wantMessage string
		wantLag     interface{}
		wantPrimary interface{}
	}{
		{"healthy", base.NewSimpleMetricResult(0.05), base.NewSimpleMetricResult(23), false, 200, "", "", 0.05, float64(23)},
		{"primary overload", base.NewSimpleMetricResult(0.05), base.NewSimpleMetricResult(80), false, 429, "mysql/primary", "Threshold exceeded", 0.05, float64(80)},
		{"primary recovery", base.NewSimpleMetricResult(0.05), base.NewErrorMetricResult(35, base.RecoveryNotCompleteError), false, 429, "mysql/primary", "Recovery period not complete", 0.05, float64(35)},
		{"primary shared health", base.NewSimpleMetricResult(0.05), base.NewSimpleMetricResult(23), true, 429, "mysql/primary", "Threshold exceeded", 0.05, float64(23)},
		{"primary probe error", base.NewSimpleMetricResult(0.05), base.NewErrorMetricResult(999, errors.New("probe failed")), false, 500, "mysql/primary", "probe failed", 0.05, float64(999)},
		{"primary no hosts", base.NewSimpleMetricResult(0.05), base.NoHostsMetricResult, false, 500, "mysql/primary", "No hosts found", 0.05, float64(0)},
		{"primary missing", base.NewSimpleMetricResult(0.05), base.NoSuchMetric, false, 500, "mysql/primary", `required MySQL cluster metric "primary" is unavailable`, 0.05, float64(0)},
		{"lag overload includes pressure", base.NewSimpleMetricResult(6), base.NewSimpleMetricResult(23), false, 429, "mysql/replicas", "Threshold exceeded", float64(6), float64(23)},
		{"lag recovery includes pressure", base.NewErrorMetricResult(2, base.RecoveryNotCompleteError), base.NewSimpleMetricResult(23), false, 429, "mysql/replicas", "Recovery period not complete", float64(2), float64(23)},
		{"both overloaded keeps first blocker", base.NewSimpleMetricResult(6), base.NewSimpleMetricResult(80), false, 429, "mysql/replicas", "Threshold exceeded", float64(6), float64(80)},
		{"lag error includes pressure", base.NewErrorMetricResult(999, errors.New("lag failed")), base.NewSimpleMetricResult(23), false, 500, "mysql/replicas", "lag failed", float64(999), float64(23)},
		{"both unavailable", base.NoMetricResultYet, base.NoMetricResultYet, false, 500, "mysql/replicas", "Metric not collected yet", float64(0), float64(0)},
		{"legacy lag no-hosts fail-open is unchanged", base.NoHostsMetricResult, base.NewSimpleMetricResult(23), false, 200, "", "No hosts found", float64(0), float64(23)},
	} {
		t.Run(test.name, func(t *testing.T) {
			throttler := NewThrottler()
			setTestMySQLClusterMetric(throttler, "replicas", 0.05, 5)
			setTestMySQLClusterMetric(throttler, "primary", 23, 50)
			setTestMySQLClusterMetric(throttler, "other", 10, 20)
			throttler.aggregatedMetrics.SetDefault("mysql/replicas", test.replica)
			throttler.aggregatedMetrics.SetDefault("mysql/primary", test.primary)
			if test.shared {
				throttler.shareDomainMetricHealth.SetDefault("mysql/primary", &base.MetricHealth{SecondsSinceLastHealthy: 1})
			}
			checked := make(map[string]bool)
			result := NewThrottlerCheck(throttler).checkMySQLCluster("test-app", "replicas", StandardCheckFlags, checked)
			assert.Equal(t, test.wantStatus, result.StatusCode)
			assert.Equal(t, "mysql/replicas", result.MetricName)
			data, err := json.Marshal(result)
			if !assert.NoError(t, err) {
				return
			}
			var body map[string]interface{}
			if !assert.NoError(t, json.Unmarshal(data, &body)) {
				return
			}
			assert.Equal(t, test.wantLag, body["Value"])
			assert.Equal(t, float64(5), body["Threshold"])
			assert.Equal(t, test.wantMessage, body["Message"])
			assert.NotContains(t, body, "MeasurementAvailable")
			assert.NotContains(t, body, "MeasurementError")
			assert.NotContains(t, body, "RejectionReason")
			if test.wantBlocker == "" {
				assert.NotContains(t, body, "BlockingMetricName")
			} else {
				assert.Equal(t, test.wantBlocker, body["BlockingMetricName"])
			}
			if !assert.Len(t, result.Dependencies, 2) {
				return
			}
			assert.Equal(t, map[string]bool{"replicas": true, "primary": true, "other": true}, checked)
			assert.Equal(t, "mysql/primary", result.Dependencies[0].MetricName)
			assert.Equal(t, 10.0, result.Dependencies[1].Value)
			dependencies := body["Dependencies"].([]interface{})
			primary := dependencies[0].(map[string]interface{})
			assert.Equal(t, test.wantPrimary, primary["Value"])
			assert.NotContains(t, primary, "MeasurementAvailable")
			assert.NotContains(t, primary, "MeasurementError")
			assert.NotContains(t, primary, "RejectionReason")
			assert.NotContains(t, primary, "BlockingMetricName")
			assert.Equal(t, result.Dependencies[0].Message, primary["Message"])
		})
	}
}

func TestDiagnosticDependenciesDoNotSetLowPriorityThrottlingState(t *testing.T) {
	original := config.Settings().Stores.MySQL
	t.Cleanup(func() { config.Settings().Stores.MySQL = original })
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas": {RequiredClusters: []string{"primary", "other"}},
			"primary":  {},
			"other":    {},
		},
	}
	for _, test := range []struct {
		name        string
		lag         float64
		blocker     string
		diagnostics []string
	}{
		{"lag blocks first", 6, "mysql/replicas", []string{"mysql/primary", "mysql/other"}},
		{"primary blocks first", 0.05, "mysql/primary", []string{"mysql/other"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			throttler := NewThrottler()
			setTestMySQLClusterMetric(throttler, "replicas", test.lag, 5)
			setTestMySQLClusterMetric(throttler, "primary", 80, 50)
			setTestMySQLClusterMetric(throttler, "other", 30, 20)
			result := NewThrottlerCheck(throttler).Check("test-app", "mysql", "replicas", "", StandardCheckFlags)
			assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
			assert.Equal(t, test.blocker, result.rejectionResult().MetricName)
			assert.Len(t, result.Dependencies, 2)
			assert.Eventually(t, func() bool {
				_, exists := throttler.nonLowPriorityAppRequestsThrottled.Get(test.blocker)
				return exists
			}, time.Second, time.Millisecond)
			assert.Never(t, func() bool {
				for _, metric := range test.diagnostics {
					if _, exists := throttler.nonLowPriorityAppRequestsThrottled.Get(metric); exists {
						return true
					}
				}
				return false
			}, 50*time.Millisecond, time.Millisecond)
		})
	}
}

func TestNestedDependenciesKeepLagAndBlockerIdentity(t *testing.T) {
	original := config.Settings().Stores.MySQL
	t.Cleanup(func() { config.Settings().Stores.MySQL = original })
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas":     {RequiredClusters: []string{"intermediate"}},
			"intermediate": {RequiredClusters: []string{"primary"}},
			"primary":      {},
		},
	}
	throttler := NewThrottler()
	setTestMySQLClusterMetric(throttler, "replicas", 0.05, 1)
	setTestMySQLClusterMetric(throttler, "intermediate", 0.1, 1)
	setTestMySQLClusterMetric(throttler, "primary", 80, 75)
	result := NewThrottlerCheck(throttler).Check("test-app", "mysql", "replicas", "", StandardCheckFlags)
	assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	assert.Equal(t, 0.05, result.Value)
	assert.Equal(t, "mysql/replicas", result.MetricName)
	if !assert.Len(t, result.Dependencies, 1) {
		return
	}
	assert.Equal(t, 0.1, result.Dependencies[0].Value)
	assert.Equal(t, "mysql/primary", result.rejectionResult().MetricName)
	assert.Equal(t, 80.0, result.rejectionResult().Value)
	data, err := json.Marshal(result)
	if !assert.NoError(t, err) {
		return
	}
	var body map[string]interface{}
	if !assert.NoError(t, json.Unmarshal(data, &body)) {
		return
	}
	assert.Equal(t, "mysql/primary", body["BlockingMetricName"])
	intermediate := body["Dependencies"].([]interface{})[0].(map[string]interface{})
	primary := intermediate["Dependencies"].([]interface{})[0].(map[string]interface{})
	for _, dependency := range []map[string]interface{}{intermediate, primary} {
		assert.NotContains(t, dependency, "BlockingMetricName")
		assert.NotContains(t, dependency, "RejectionReason")
		assert.NotContains(t, dependency, "MeasurementError")
	}

	throttler.aggregatedMetrics.Delete("mysql/intermediate")
	result = NewThrottlerCheck(throttler).Check("test-app", "mysql", "replicas", "", StandardCheckFlags)
	assert.Equal(t, http.StatusInternalServerError, result.StatusCode)
	assert.Equal(t, 0.05, result.Value)
	assert.Equal(t, "mysql/intermediate", result.rejectionResult().MetricName)
	if !assert.Len(t, result.Dependencies, 1) || !assert.Len(t, result.Dependencies[0].Dependencies, 1) {
		return
	}
	assert.Equal(t, 80.0, result.Dependencies[0].Dependencies[0].Value)
}
