package throttle_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	frenohttp "github.com/github/freno/pkg/http"
	"github.com/github/freno/pkg/throttle"
)

func requestHTTPCheck(t *testing.T, server *httptest.Server) (int, throttle.CheckResult) {
	t.Helper()
	response, err := server.Client().Get(server.URL + "/check/test-app/mysql/replicas")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result throttle.CheckResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != response.StatusCode {
		t.Fatalf("HTTP status %d differs from response status %d", response.StatusCode, result.StatusCode)
	}
	return response.StatusCode, result
}

func TestHTTPMySQLPrimaryDependencies(t *testing.T) {
	for _, test := range []struct {
		name            string
		dependencyField string
		wantStatus      int
		wantMetric      string
		wantValue       float64
		wantThreshold   float64
	}{
		{"required primary blocks", "RequiredClusters", http.StatusTooManyRequests, "mysql/replicas", 1, 5},
		{"removed shadow field is ignored", "ShadowRequiredClusters", http.StatusOK, "mysql/replicas", 1, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer config.Reset()
			directory := t.TempDir()
			contents := fmt.Sprintf(`{
				"RaftDataDir": %q,
				"Stores": {"MySQL": {"Clusters": {
					"replicas": {"ThrottleThreshold": 5, %q: ["primary"]},
					"primary": {"ThrottleThreshold": 50}
				}}}
			}`, directory, test.dependencyField)
			filename := filepath.Join(directory, "freno.json")
			if err := os.WriteFile(filename, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			if err := config.Instance().Read(filename); err != nil {
				t.Fatal(err)
			}

			throttler := throttle.NewThrottler()
			throttle.SetTestMySQLClusterMetric(throttler, "replicas", 1, 5)
			throttle.SetTestMySQLClusterMetric(throttler, "primary", 60, 50)
			router := frenohttp.ConfigureRoutes(frenohttp.NewAPIImpl(throttle.NewThrottlerCheck(throttler), nil))
			server := httptest.NewServer(router)
			defer server.Close()

			status, result := requestHTTPCheck(t, server)
			if status != test.wantStatus || result.MetricName != test.wantMetric ||
				result.Value != test.wantValue || result.Threshold != test.wantThreshold {
				t.Fatalf("unexpected HTTP check: %+v", result)
			}
			t.Logf("%s: HTTP %d, metric=%s, value=%g, threshold=%g",
				test.dependencyField, status, result.MetricName, result.Value, result.Threshold)
		})
	}
}

func TestHTTPMySQLPrimaryRecoveryAfterProbeError(t *testing.T) {
	defer config.Reset()
	recoveryThreshold := 40.0
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas": {ThrottleThreshold: 5, RequiredClusters: []string{"primary"}},
			"primary": {
				ThrottleThreshold:      50,
				RecoveryThreshold:      &recoveryThreshold,
				RecoveryDurationMillis: 5000,
				FailOnNoHosts:          true,
			},
		},
	}
	throttler := throttle.NewThrottler()
	throttle.SetTestMySQLClusterMetric(throttler, "replicas", 1, 5)
	throttle.SetTestMySQLClusterMetric(throttler, "primary", 60, 50)
	router := frenohttp.ConfigureRoutes(frenohttp.NewAPIImpl(throttle.NewThrottlerCheck(throttler), nil))
	server := httptest.NewServer(router)
	defer server.Close()

	start := time.Date(2026, time.November, 16, 0, 0, 0, 0, time.UTC)
	probeError := errors.New("test probe failed")
	for _, step := range []struct {
		name        string
		elapsed     time.Duration
		metric      base.MetricResult
		wantStatus  int
		wantMessage string
	}{
		{"overloaded", 0, base.NewSimpleMetricResult(60), http.StatusTooManyRequests, base.ThresholdExceededError.Error()},
		{"healthy window starts", time.Second, base.NewSimpleMetricResult(35), http.StatusTooManyRequests, base.RecoveryNotCompleteError.Error()},
		{"healthy window progresses", 3 * time.Second, base.NewSimpleMetricResult(35), http.StatusTooManyRequests, base.RecoveryNotCompleteError.Error()},
		{"probe fails", 4 * time.Second, base.NewErrorMetricResult(0, probeError), http.StatusInternalServerError, probeError.Error()},
		{"fresh healthy window starts", 5 * time.Second, base.NewSimpleMetricResult(35), http.StatusTooManyRequests, base.RecoveryNotCompleteError.Error()},
		{"original deadline is not sufficient", 6 * time.Second, base.NewSimpleMetricResult(35), http.StatusTooManyRequests, base.RecoveryNotCompleteError.Error()},
		{"fresh window not yet complete", 10*time.Second - time.Millisecond, base.NewSimpleMetricResult(35), http.StatusTooManyRequests, base.RecoveryNotCompleteError.Error()},
		{"fresh full window complete", 10 * time.Second, base.NewSimpleMetricResult(35), http.StatusOK, ""},
	} {
		t.Run(step.name, func(t *testing.T) {
			throttle.StabilizeTestMySQLClusterMetric(throttler, "primary", step.metric, start.Add(step.elapsed))
			status, result := requestHTTPCheck(t, server)
			if status != step.wantStatus || result.Message != step.wantMessage {
				t.Fatalf("at %s: unexpected HTTP check: %+v", step.elapsed, result)
			}
			wantMetric := "mysql/replicas"
			if result.MetricName != wantMetric {
				t.Fatalf("metric = %q, want %q", result.MetricName, wantMetric)
			}
			if result.Value != 1 || result.Threshold != 5 || len(result.Dependencies) != 1 {
				t.Fatalf("lag fields or primary measurement lost: %+v", result)
			}
			value, _ := step.metric.Get()
			if result.Dependencies[0].Value != value {
				t.Fatalf("primary value = %g, want %g", result.Dependencies[0].Value, value)
			}
			t.Logf("at %s: HTTP %d, metric=%s, message=%q", step.elapsed, status, result.MetricName, result.Message)
		})
	}
}
