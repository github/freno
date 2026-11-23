package throttle_test

import (
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	frenohttp "github.com/github/freno/pkg/http"
	"github.com/github/freno/pkg/throttle"
)

func TestRubyClientLagPressureCompatibility(t *testing.T) {
	clientPath := os.Getenv("FRENO_RUBY_CLIENT_PATH")
	if clientPath == "" {
		t.Skip("set FRENO_RUBY_CLIENT_PATH to an unchanged freno-client checkout and provide Ruby with Faraday")
	}
	ruby, err := exec.LookPath("ruby")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(clientPath, "lib", "freno", "client.rb")); err != nil {
		t.Fatal(err)
	}

	defer config.Reset()
	recoveryThreshold := 40.0
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"replicas": {ThrottleThreshold: 1, RequiredClusters: []string{"primary"}},
			"primary": {
				ThrottleThreshold:      75,
				RecoveryThreshold:      &recoveryThreshold,
				RecoveryDurationMillis: 1000,
				FailOnNoHosts:          true,
			},
		},
	}
	throttler := throttle.NewThrottler()
	throttle.SetTestMySQLClusterMetric(throttler, "replicas", 0.05, 1)
	throttle.SetTestMySQLClusterMetric(throttler, "primary", 80, 75)
	router := frenohttp.ConfigureRoutes(frenohttp.NewAPIImpl(throttle.NewThrottlerCheck(throttler), nil))
	server := httptest.NewServer(router)
	defer server.Close()

	const script = `
require "faraday"
require "freno/client"

url, expected_status = ARGV
connection = Faraday.new(url: url) do |builder|
  builder.options.open_timeout = 2
  builder.options.timeout = 2
end
client = Freno::Client.new(connection) do |config|
  config.default_app = "test-app"
  config.default_store_name = "replicas"
end
delay = client.replication_delay
raise "expected lag seconds, got #{delay.inspect}" unless delay == 0.05

# The application's default_replication_wait! calculation uses the maximum
# cluster delay, converts seconds to milliseconds and adds 100ms.
wait_millis = [delay].max * 1000 + 100
raise "incorrect application wait: #{wait_millis}" unless wait_millis == 150

result = client.check
raise "incorrect admission status: #{result.code}" unless result.code == expected_status.to_i
raise "incorrect admission boolean" unless client.check? == (expected_status == "200")

body = JSON.parse(connection.get("/check/test-app/mysql/replicas").body)
raise "requested metric replaced" unless body["MetricName"] == "mysql/replicas"
raise "missing pressure" unless body.fetch("Dependencies").first["MetricName"] == "mysql/primary"
puts "lag=#{delay}s wait=#{wait_millis}ms admission=#{result.code}"
`
	start := time.Date(2026, time.November, 23, 0, 0, 0, 0, time.UTC)
	for _, step := range []struct {
		name    string
		elapsed time.Duration
		metric  base.MetricResult
		status  string
		shared  int64
	}{
		{"primary overloaded", 0, base.NewSimpleMetricResult(80), "429", 0},
		{"recovering", time.Second, base.NewSimpleMetricResult(35), "429", 0},
		{"healthy", 2 * time.Second, base.NewSimpleMetricResult(35), "200", 0},
		{"primary probe fails", 3 * time.Second, base.NewErrorMetricResult(0, errors.New("probe failed")), "500", 0},
		{"fresh recovery", 4 * time.Second, base.NewSimpleMetricResult(35), "429", 0},
		{"recovered after probe failure", 5 * time.Second, base.NewSimpleMetricResult(35), "200", 0},
		{"shared domain unhealthy", 6 * time.Second, base.NewSimpleMetricResult(35), "429", 1},
		{"shared domain healthy again", 7 * time.Second, base.NewSimpleMetricResult(35), "200", 0},
	} {
		t.Run(step.name, func(t *testing.T) {
			throttle.StabilizeTestMySQLClusterMetric(throttler, "primary", step.metric, start.Add(step.elapsed))
			throttle.SetTestMySQLSharedHealth(throttler, "primary", step.shared)
			command := exec.Command(ruby, "-I"+filepath.Join(clientPath, "lib"), "-e", script, server.URL, step.status)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("unchanged Ruby client failed: %v\n%s", err, output)
			}
			t.Log(string(output))
		})
	}
}
