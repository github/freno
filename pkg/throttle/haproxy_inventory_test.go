package throttle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/mysql"
)

func TestRefreshMySQLInventoryHAProxyServerAddress(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() { config.Settings().Stores.MySQL = originalSettings }()

	tests := []struct {
		name        string
		options     string
		csv         string
		ignoreHosts []string
		skippedHost string
		want        map[string][]mysql.InstanceKey
	}{
		{
			name:    "default preserves hostname identity and exclusions",
			options: `{}`,
			csv: "# pxname,svname,status,addr\n" +
				"writer_a,db-a.example.com,no check,192.0.2.10:3307\n" +
				"writer_a,ignore-me.example.com,UP,192.0.2.11:3308\n",
			ignoreHosts: []string{"ignore-me"},
			want: map[string][]mysql.InstanceKey{
				"writer_a": {{Hostname: "db-a.example.com", Port: 3316}},
			},
		},
		{
			name:    "disabled supports CSV without addr",
			options: `{"UseServerAddress":false}`,
			csv:     "# pxname,svname,status\nwriter_a,db-a.example.com,no check\n",
			want: map[string][]mysql.InstanceKey{
				"writer_a": {{Hostname: "db-a.example.com", Port: 3316}},
			},
		},
		{
			name:    "opt in keeps identical labels in different pools distinct",
			options: `{"UseServerAddress":true}`,
			csv: "# pxname,svname,status,addr\n" +
				"writer_a,primary,no check,192.0.2.10:3307\n" +
				"writer_b,primary,no check,192.0.2.11:3308\n",
			want: map[string][]mysql.InstanceKey{
				"writer_a": {{Hostname: "192.0.2.10", Port: 3307}},
				"writer_b": {{Hostname: "192.0.2.11", Port: 3308}},
			},
		},
		{
			name:    "opt in supports IPv6",
			options: `{"UseServerAddress":true}`,
			csv:     "# pxname,svname,status,addr\nwriter_a,primary,no check,[2001:db8::10]:3309\n",
			want: map[string][]mysql.InstanceKey{
				"writer_a": {{Hostname: "2001:db8::10", Port: 3309}},
			},
		},
		{
			name:    "opt in applies exclusions to discovered addresses",
			options: `{"UseServerAddress":true}`,
			csv: "# pxname,svname,status,addr\n" +
				"writer_a,primary,no check,192.0.2.10:3307\n" +
				"writer_a,secondary,UP,192.0.2.11:3308\n",
			ignoreHosts: []string{"192.0.2.10"},
			skippedHost: "192.0.2.11",
			want:        map[string][]mysql.InstanceKey{"writer_a": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.csv)
			}))
			defer server.Close()

			settings := config.MySQLConfigurationSettings{
				Clusters: make(map[string]*config.MySQLClusterConfigurationSettings),
			}
			for clusterName := range tt.want {
				cluster := &config.MySQLClusterConfigurationSettings{
					Port:        3316,
					IgnoreHosts: tt.ignoreHosts,
					HAProxySettings: config.HAProxyConfigurationSettings{
						Addresses: server.URL,
						PoolName:  clusterName,
					},
				}
				if err := json.Unmarshal([]byte(tt.options), &cluster.HAProxySettings); err != nil {
					t.Fatal(err)
				}
				settings.Clusters[clusterName] = cluster
			}
			config.Settings().Stores.MySQL = settings
			throttler := NewThrottler()
			throttler.isLeader = true
			throttler.mysqlClusterProbesChan = make(chan *mysql.ClusterProbes, len(tt.want))
			if tt.skippedHost != "" {
				throttler.skippedHosts.SetDefault(tt.skippedHost, true)
			}
			if err := throttler.refreshMySQLInventory(); err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]bool)
			for range tt.want {
				select {
				case probes := <-throttler.mysqlClusterProbesChan:
					want, ok := tt.want[probes.ClusterName]
					if !ok || seen[probes.ClusterName] {
						t.Fatalf("unexpected cluster probes: %s", probes.ClusterName)
					}
					seen[probes.ClusterName] = true
					if got := len(*probes.InstanceProbes); got != len(want) {
						t.Errorf("%s probe count = %d, want %d", probes.ClusterName, got, len(want))
					}
					for _, key := range want {
						if probe := (*probes.InstanceProbes)[key]; probe == nil || probe.Key != key {
							t.Errorf("%s missing probe for %+v; got %+v", probes.ClusterName, key, *probes.InstanceProbes)
						}
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for HAProxy probes")
				}
			}
		})
	}
}

func TestRefreshMySQLInventoryHAProxyServerAddressChanges(t *testing.T) {
	originalSettings := config.Settings().Stores.MySQL
	defer func() { config.Settings().Stores.MySQL = originalSettings }()

	var response atomic.Value
	setAddress := func(address string) {
		response.Store("# pxname,svname,status,addr\nwriters,primary,no check," + address + "\n")
	}
	setAddress("192.0.2.10:3307")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, response.Load().(string))
	}))
	defer server.Close()
	cluster := &config.MySQLClusterConfigurationSettings{
		Port: 3306,
		HAProxySettings: config.HAProxyConfigurationSettings{
			Addresses: server.URL,
			PoolName:  "writers",
		},
	}
	if err := json.Unmarshal([]byte(`{"UseServerAddress":true}`), &cluster.HAProxySettings); err != nil {
		t.Fatal(err)
	}
	config.Settings().Stores.MySQL = config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{"writers": cluster},
	}
	throttler := NewThrottler()
	throttler.isLeader = true
	throttler.mysqlClusterProbesChan = make(chan *mysql.ClusterProbes, 1)

	refresh := func(want mysql.InstanceKey) {
		t.Helper()
		if err := throttler.refreshMySQLInventory(); err != nil {
			t.Fatal(err)
		}
		select {
		case probes := <-throttler.mysqlClusterProbesChan:
			if err := throttler.updateMySQLClusterProbes(probes); err != nil {
				t.Fatal(err)
			}
			got := throttler.mysqlInventory.ClustersProbes["writers"]
			if got == nil || len(*got) != 1 || (*got)[want] == nil {
				t.Fatalf("inventory = %+v, want only %+v", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for refreshed HAProxy probes")
		}
	}
	refresh(mysql.InstanceKey{Hostname: "192.0.2.10", Port: 3307})
	setAddress("[2001:db8::11]:3308")
	time.Sleep(1100 * time.Millisecond) // HAProxy responses are cached for one second.
	refresh(mysql.InstanceKey{Hostname: "2001:db8::11", Port: 3308})
}
