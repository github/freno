package http

import (
	"encoding/json"
	"expvar"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/group"
	"github.com/github/freno/pkg/throttle"
	metrics "github.com/rcrowley/go-metrics"
)

type metricsConsensusService struct {
	group.ConsensusService
	isLeader bool
}

func (s *metricsConsensusService) IsLeader() bool {
	return s.isLeader
}

func TestLbCheck(t *testing.T) {
	api := NewAPIImpl(nil, nil)
	recorder := httptest.NewRecorder()
	api.LbCheck(recorder, &http.Request{}, nil)

	code, body := recorder.Code, recorder.Body.String()

	if code != http.StatusOK {
		t.Errorf("Expected LbCheck to respond with %d status code, but responded with %d", http.StatusOK, code)
	}

	if len(body) > 0 {
		t.Errorf("Expected LbCheck to respond with empty body, but responded with %s", body)
	}
}

// TestRoutes applies an end-to-end canary test over each of the different routes
func TestRoutes(t *testing.T) {
	router := ConfigureRoutes(new(APIImpl))

	expectedRoutes := []struct {
		verb string
		path string
		code int
	}{
		{http.MethodGet, "/lb-check", http.StatusOK},
		{http.MethodGet, "/config/memcache", http.StatusOK},
		{http.MethodGet, "/debug/metrics", http.StatusOK},
	}
	for _, route := range expectedRoutes {
		r, _ := http.NewRequest(route.verb, route.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if !(w.Code == route.code) {
			t.Errorf("Route %s failed: code {expected=%d, actual=%d}", route.path, route.code, w.Code)
		}
	}
}

func TestExplainMySQLRoute(t *testing.T) {
	defer config.Reset()
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		User: "private-user", Password: "private-password",
		ProxySQLPassword: "private-proxy-password",
		MetricQuery:      "private-query",
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"source": {
				ThrottleThreshold:      1,
				ShadowRequiredClusters: []string{"source-primary"},
			},
			"source-primary": {
				User: "private-cluster-user", Password: "private-cluster-password",
				MetricQuery: "private-cluster-query", ThrottleThreshold: 75,
				StaticHostsSettings: config.StaticHostsConfigurationSettings{Hosts: []string{"private-host"}},
			},
			"broken": {RequiredClusters: []string{"not-configured"}},
		},
	}
	for _, test := range []struct {
		name     string
		leader   bool
		cluster  string
		wantCode int
	}{
		{"leader snapshot", true, "source", http.StatusOK},
		{"follower", false, "source", http.StatusServiceUnavailable},
		{"unknown cluster", true, "unknown", http.StatusNotFound},
		{"invalid dependencies", true, "broken", http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			check := throttle.NewThrottlerCheck(throttle.NewThrottler())
			api := NewAPIImpl(check, &metricsConsensusService{isLeader: test.leader})
			router := ConfigureRoutes(api)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/explain/mysql/"+test.cluster, nil))
			if recorder.Code != test.wantCode {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.wantCode, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private-") {
				t.Fatal("diagnostic response exposed private configuration")
			}
			if test.wantCode == http.StatusOK {
				var result throttle.MySQLExplanation
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Metrics) != 2 || result.Metrics[1].Threshold != 75 ||
					result.Metrics[1].Mode != "shadow" || result.Metrics[1].Value != nil {
					t.Fatalf("unexpected diagnostic response: %s", recorder.Body.String())
				}
				if !strings.Contains(recorder.Body.String(), `"Value":null`) {
					t.Fatal("missing measurement must be JSON null")
				}
			}
		})
	}
}

func TestMemcacheConfigWhenProvided(t *testing.T) {
	defer config.Reset()

	api := NewAPIImpl(nil, nil)
	recorder := httptest.NewRecorder()
	settings := config.Settings()
	settings.MemcacheServers = []string{"memcache.server.one:11211", "memcache.server.two:11211"}
	settings.MemcachePath = "myCacheNamespace"

	api.MemcacheConfig(recorder, &http.Request{}, nil)

	code, body := recorder.Code, recorder.Body.String()
	if code != http.StatusOK {
		t.Errorf("Expected MemcacheConfig to respond with %d status code, but responded with %d", http.StatusOK, code)
	}

	type memcacheSettings struct {
		MemcacheServers []string
		MemcachePath    string
	}

	var expected, actual memcacheSettings
	json.Unmarshal([]byte(`{"MemcacheServers":["memcache.server.one:11211","memcache.server.two:11211"],"MemcachePath":"myCacheNamespace"}`), &expected)
	json.Unmarshal([]byte(body), &actual)

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected MemcacheConfig body to be %s, but it's %s", expected, body)
	}
}

func TestMemcacheConfigWhenDefault(t *testing.T) {
	api := NewAPIImpl(nil, nil)
	recorder := httptest.NewRecorder()
	api.MemcacheConfig(recorder, &http.Request{}, nil)

	code, body := recorder.Code, recorder.Body.String()
	if code != http.StatusOK {
		t.Errorf("Expected MemcacheConfig to respond with %d status code, but responded with %d", http.StatusOK, code)
	}

	type memcacheSettings struct {
		MemcacheServers []string
		MemcachePath    string
	}

	var expected, actual memcacheSettings
	json.Unmarshal([]byte(`{"MemcacheServers":[],"MemcachePath":"freno"}`), &expected)
	json.Unmarshal([]byte(body), &actual)

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected MemcacheConfig body to be %s, but it's %s", expected, body)
	}
}

func TestMetricsFiltersAggregateAndProbeMetricsFromFollowers(t *testing.T) {
	const (
		aggregatedMetricName = "aggregated.mysql.metrics-export-test"
		probeMetricName      = "probes.total.metrics-export-test"
		processMetricName    = "consensus.metrics-export-test"
		backgroundMetricName = "shadow.background.mysql.metrics-export-test.would_reject"
		httpMetricName       = "shadow.http.mysql.metrics-export-test.total"
	)

	metricNames := []string{aggregatedMetricName, probeMetricName, processMetricName, backgroundMetricName, httpMetricName}
	for _, metricName := range metricNames {
		metrics.DefaultRegistry.Unregister(metricName)
		defer metrics.DefaultRegistry.Unregister(metricName)
	}

	metrics.GetOrRegisterGaugeFloat64(aggregatedMetricName, nil).Update(42)
	metrics.GetOrRegisterCounter(probeMetricName, nil).Inc(7)
	metrics.GetOrRegisterGauge(processMetricName, nil).Update(1)
	metrics.GetOrRegisterCounter(backgroundMetricName, nil).Inc(2)
	metrics.GetOrRegisterCounter(httpMetricName, nil).Inc(3)

	readMetrics := func(isLeader bool) map[string]interface{} {
		recorder := httptest.NewRecorder()
		api := NewAPIImpl(nil, &metricsConsensusService{isLeader: isLeader})
		api.Metrics(recorder, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil), nil)
		if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
			t.Errorf("Unexpected content type: %s", contentType)
		}
		result := make(map[string]interface{})
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	leaderMetrics := readMetrics(true)
	metrics.DefaultRegistry.Unregister(aggregatedMetricName)
	metrics.DefaultRegistry.Unregister(probeMetricName)
	metrics.DefaultRegistry.Unregister(backgroundMetricName)
	if expvar.Get(aggregatedMetricName) == nil || expvar.Get(probeMetricName) == nil {
		t.Fatal("Expected aggregate and probe metrics to remain in expvar after unregister")
	}

	followerMetrics := readMetrics(false)
	if leaderMetrics[aggregatedMetricName] == nil || leaderMetrics[probeMetricName] == nil || leaderMetrics[processMetricName] == nil {
		t.Errorf("Unexpected leader metrics: %v", leaderMetrics)
	}
	if followerMetrics[aggregatedMetricName] != nil || followerMetrics[probeMetricName] != nil || followerMetrics[processMetricName] == nil {
		t.Errorf("Unexpected follower metrics: %v", followerMetrics)
	}
	if leaderMetrics[backgroundMetricName] == nil || leaderMetrics[httpMetricName] == nil ||
		followerMetrics[backgroundMetricName] != nil || followerMetrics[httpMetricName] == nil {
		t.Errorf("Unexpected shadow metrics: leader=%v follower=%v", leaderMetrics, followerMetrics)
	}
	if leaderMetrics[backgroundMetricName] != float64(2) || leaderMetrics[httpMetricName] != float64(3) ||
		followerMetrics[httpMetricName] != float64(3) {
		t.Errorf("Expected integer shadow counter values, got leader=%v follower=%v", leaderMetrics, followerMetrics)
	}
}
