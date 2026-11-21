package throttle

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/outbrain/golib/log"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
)

func captureSafetyLogs(t *testing.T) func() string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "safety-logs")
	if err != nil {
		t.Fatal(err)
	}
	stderr, level := os.Stderr, log.GetLevel()
	os.Stderr = file
	log.SetLevel(log.INFO)
	t.Cleanup(func() {
		os.Stderr = stderr
		log.SetLevel(level)
		file.Close()
	})
	return func() string {
		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

func clearSafetyCounters(t *testing.T) {
	t.Helper()
	clear := func() {
		var names []string
		metrics.DefaultRegistry.Each(func(name string, _ interface{}) {
			if strings.HasPrefix(name, "check.rejection.") || strings.HasPrefix(name, "check.recovery.") {
				names = append(names, name)
			}
		})
		for _, name := range names {
			metrics.DefaultRegistry.Unregister(name)
		}
	}
	clear()
	t.Cleanup(clear)
}

func TestSafetyRejectionTelemetry(t *testing.T) {
	original := config.Settings().Stores.MySQL
	t.Cleanup(func() { config.Settings().Stores.MySQL = original })
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		FallbackCluster: "telemetry-replicas",
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"telemetry-replicas": {RequiredClusters: []string{"telemetry-primary"}},
			"telemetry-primary":  {FailOnNoHosts: true},
		},
	}
	for _, test := range []struct {
		name       string
		metric     base.MetricResult
		replica    float64
		shared     bool
		app        string
		requested  string
		wantStatus int
		wantReason string
		dependency string
	}{
		{name: "accepted", metric: base.NewSimpleMetricResult(10), wantStatus: http.StatusOK},
		{name: "primary threshold", metric: base.NewSimpleMetricResult(60), wantStatus: 429, wantReason: "threshold"},
		{name: "first blocker only", replica: 6, metric: base.NewSimpleMetricResult(60), wantStatus: 429, wantReason: "threshold", dependency: "telemetry-replicas"},
		{name: "recovery", metric: base.NewErrorMetricResult(35, base.RecoveryNotCompleteError), wantStatus: 429, wantReason: "recovery"},
		{name: "shared domain", shared: true, metric: base.NewSimpleMetricResult(10), wantStatus: 429, wantReason: "shared_domain"},
		{name: "missing measurement", metric: base.NoSuchMetric, wantStatus: 500, wantReason: "missing_metric"},
		{name: "no hosts", metric: base.NoHostsMetricResult, wantStatus: 500, wantReason: "no_hosts"},
		{name: "metric error", metric: base.NewErrorMetricResult(0, errors.New("private connection details")), wantStatus: 500, wantReason: "metric_error"},
		{name: "fallback is bounded", requested: "unbounded-caller-input", metric: base.NewSimpleMetricResult(60), wantStatus: 429, wantReason: "threshold"},
		{name: "internal check excluded", app: frenoAppName, metric: base.NewSimpleMetricResult(60), wantStatus: 429},
		{name: "shared internal check excluded", app: frenoShareDmainAppName, metric: base.NewSimpleMetricResult(60), wantStatus: 429},
		{name: "application denial excluded", app: "abusing-app", metric: base.NewSimpleMetricResult(10), wantStatus: 417},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearSafetyCounters(t)
			logs := captureSafetyLogs(t)
			throttler := NewThrottler()
			setTestMySQLClusterMetric(throttler, "telemetry-replicas", test.replica, 5)
			setTestMySQLClusterMetric(throttler, "telemetry-primary", 10, 50)
			throttler.aggregatedMetrics.SetDefault("mysql/telemetry-primary", test.metric)
			if test.shared {
				throttler.shareDomainMetricHealth.SetDefault("mysql/telemetry-primary", &base.MetricHealth{SecondsSinceLastHealthy: 1})
			}
			app, requested := test.app, test.requested
			if app == "" {
				app = "test-app"
			}
			if requested == "" {
				requested = "telemetry-replicas"
			}
			check := NewThrottlerCheck(throttler)
			for attempt := 0; attempt < 2; attempt++ {
				result := check.Check(app, "mysql", requested, "", StandardCheckFlags)
				assert.Equal(t, test.wantStatus, result.StatusCode)
			}
			var counters []string
			metrics.DefaultRegistry.Each(func(name string, _ interface{}) {
				if strings.HasPrefix(name, "check.rejection.") {
					counters = append(counters, name)
				}
			})
			if test.wantReason == "" {
				assert.Empty(t, counters)
				assert.NotContains(t, logs(), "event=freno_admission_rejected")
				return
			}
			dependency := test.dependency
			if dependency == "" {
				dependency = "telemetry-primary"
			}
			name := fmt.Sprintf("check.rejection.mysql.telemetry-replicas.dependency.%s.status.%d.%s.total", dependency, test.wantStatus, test.wantReason)
			if !assert.Equal(t, []string{name}, counters) {
				return
			}
			assert.Equal(t, int64(2), metrics.Get(name).(metrics.Counter).Count())
			assert.Equal(t, 1, strings.Count(logs(), "event=freno_admission_rejected"))
			assert.Contains(t, logs(), fmt.Sprintf("reason=%q", test.wantReason))
			assert.Contains(t, logs(), fmt.Sprintf("dependency=%q", dependency))
			assert.NotContains(t, logs(), "private connection details")
			assert.NotContains(t, logs(), "unbounded-caller-input")
			// The next interval logs again without suppressing any request counters.
			throttler.safetyRejectionLogs.Delete(name)
			check.Check(app, "mysql", requested, "", StandardCheckFlags)
			assert.Equal(t, int64(3), metrics.Get(name).(metrics.Counter).Count())
			assert.Equal(t, 2, strings.Count(logs(), "event=freno_admission_rejected"))
		})
	}
}

func TestRecoveryTransitionTelemetry(t *testing.T) {
	clearSafetyCounters(t)
	logs := captureSafetyLogs(t)
	original := config.Settings().Stores.MySQL
	t.Cleanup(func() { config.Settings().Stores.MySQL = original })
	recoveryThreshold := 40.0
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"telemetry-primary": {
				ThrottleThreshold: 50, RecoveryThreshold: &recoveryThreshold,
				RecoveryDurationMillis: 5000, FailOnNoHosts: true,
			},
		},
	}
	throttler := NewThrottler()
	start := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	for _, step := range []struct {
		second int
		value  float64
		err    error
		want   error
	}{
		{0, 30, nil, base.RecoveryNotCompleteError},
		{1, 30, nil, base.RecoveryNotCompleteError},
		{2, 45, nil, base.RecoveryNotCompleteError},
		{3, 30, nil, base.RecoveryNotCompleteError},
		{4, 0, base.NoHostsError, base.NoHostsError},
		{5, 0, base.NoHostsError, base.NoHostsError},
		{6, 30, nil, base.RecoveryNotCompleteError},
		{11, 30, nil, nil},
		{12, 60, nil, nil},
		{13, 60, nil, nil},
		{14, 30, nil, base.RecoveryNotCompleteError},
		{19, 30, nil, nil},
	} {
		result := throttler.stabilizeMySQLMetric("telemetry-primary", base.NewErrorMetricResult(step.value, step.err), start.Add(time.Duration(step.second)*time.Second))
		value, err := result.Get()
		assert.Equal(t, step.value, value)
		assert.Equal(t, step.want, err)
	}
	throttler.isLeader = true
	throttler.updateLeadership(false)
	throttler.updateLeadership(true)
	result := throttler.stabilizeMySQLMetric("telemetry-primary", base.NewSimpleMetricResult(30), start.Add(20*time.Second))
	_, err := result.Get()
	assert.ErrorIs(t, err, base.RecoveryNotCompleteError)
	for event, want := range map[string]int64{"initialized": 2, "started": 5, "reset": 2, "completed": 2, "blocked": 1} {
		name := "check.recovery.mysql.telemetry-primary." + event + ".total"
		if !assert.NotNil(t, metrics.Get(name), name) {
			continue
		}
		assert.Equal(t, want, metrics.Get(name).(metrics.Counter).Count(), event)
		assert.Equal(t, int(want), strings.Count(logs(), "event=freno_recovery_"+event+" "), event)
	}
	assert.Contains(t, logs(), `reason="above_recovery_threshold"`)
	assert.Contains(t, logs(), `reason="no_hosts"`)
	assert.Contains(t, logs(), "recovery_duration_ms=5000")
}
