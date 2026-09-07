package throttle_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/freno/pkg/config"
	frenohttp "github.com/github/freno/pkg/http"
	"github.com/github/freno/pkg/throttle"
)

func TestFallbackHTTPCheckEndpoints(t *testing.T) {
	defer config.Reset()
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		FallbackCluster: "fallback",
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"fallback": {},
		},
	}

	throttler := throttle.NewThrottler()
	throttle.SetTestMySQLClusterMetric(throttler, "fallback", 10.0, 5.0)
	router := frenohttp.ConfigureRoutes(frenohttp.NewAPIImpl(throttle.NewThrottlerCheck(throttler), nil))

	tests := []struct {
		path string
		want int
	}{
		{path: "/check/test-app/mysql/unknown", want: http.StatusTooManyRequests},
		{path: "/check-read/test-app/mysql/unknown/20", want: http.StatusOK},
		{path: "/check-if-exists/test-app/mysql/unknown", want: http.StatusOK},
		{path: "/check-read-if-exists/test-app/mysql/unknown/5", want: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d; body: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}
