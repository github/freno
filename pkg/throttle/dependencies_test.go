package throttle

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/mysql"
	"github.com/stretchr/testify/assert"
)

func TestRequiredMySQLMemcachePublication(t *testing.T) {
	for _, enforce := range []bool{false, true} {
		t.Run(fmt.Sprintf("enforced=%t", enforce), func(t *testing.T) {
			original := config.Settings().Stores.MySQL
			t.Cleanup(func() { config.Settings().Stores.MySQL = original })
			root := &config.MySQLClusterConfigurationSettings{ThrottleThreshold: 5}
			if enforce {
				root.RequiredClusters = []string{"primary"}
			}
			config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
				Clusters: map[string]*config.MySQLClusterConfigurationSettings{
					"root": root, "primary": {ThrottleThreshold: 20, FailOnNoHosts: true},
				},
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
			throttler := NewThrottler()
			throttler.memcacheClient = memcache.New(listener.Addr().String())
			throttler.memcachePath = "dependencies-test"
			throttler.isLeader = true
			key := mysql.InstanceKey{Hostname: "root", Port: 3306}
			probes := mysql.Probes{key: &mysql.Probe{Key: key}}
			throttler.mysqlInventory.ClustersProbes["root"] = &probes
			throttler.mysqlInventory.InstanceKeyMetrics[mysql.GetClusterInstanceKey("root", &key)] = base.NewSimpleMetricResult(1)
			assert.NoError(t, throttler.aggregateMySQLMetrics())
			select {
			case result := <-published:
				assert.NoError(t, result.err)
				want := "set dependencies-test/mysql/root "
				if enforce {
					want = "delete dependencies-test/mysql/root"
				}
				assert.True(t, strings.HasPrefix(result.command, want), result.command)
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for memcache publication")
			}
		})
	}
}
