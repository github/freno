package proxysql

import (
	"errors"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/github/freno/pkg/config"
)

// Run only against testdata/capacity.cnf in an isolated container network.
func TestCapacityIntegrationNoBackendConnections(t *testing.T) {
	if os.Getenv("FRENO_PROXYSQL_CAPACITY_INTEGRATION") != "1" {
		t.Skip("requires the isolated ProxySQL capacity fixture")
	}
	backend, err := net.Listen("tcp", "127.0.0.1:3407")
	if err != nil {
		t.Fatal(err)
	}
	var connections int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := backend.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					t.Error(err)
				}
				return
			}
			atomic.AddInt64(&connections, 1)
			connection.Close()
		}
	}()
	defer func() {
		backend.Close()
		<-done
	}()
	deadline := time.Now().Add(15 * time.Second)
	for {
		connection, err := net.DialTimeout("tcp", "127.0.0.1:6032", time.Second)
		if err == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ProxySQL fixture did not become ready: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	sampler := NewCapacitySampler()
	defer sampler.Close()
	now := time.Now()
	sampler.now = func() time.Time { return now }
	settings := config.ProxySQLCapacityConfigurationSettings{
		Metric: "frontend", FrontendMaxConnections: 200000, WriterHostgroupID: 30,
		BackendLimits: []config.ProxySQLBackendLimit{{Host: "127.0.0.1", Port: 3407, MaxConnections: 1000}},
	}
	for cycle := 0; cycle < 3; cycle++ {
		for _, metric := range []string{"frontend", "writer-backend"} {
			settings.Metric = metric
			if value, err := sampler.Read("127.0.0.1", 6032, "stats", "capacity-test-only", settings); err != nil || value != 0 {
				t.Fatalf("%s = %v, %v; want an idle fixture", metric, value, err)
			}
		}
		now = now.Add(CapacityPollInterval)
	}
	if count := atomic.LoadInt64(&connections); count != 0 {
		t.Fatalf("stats sampling opened %d backend connections; want zero", count)
	}
}
