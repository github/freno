package config

import (
	"strings"
	"testing"
)

func capacityClusterSettings() *MySQLClusterConfigurationSettings {
	recovery := 70.0
	return &MySQLClusterConfigurationSettings{
		User: "stats", Password: "test-only", ThrottleThreshold: 90,
		RecoveryThreshold: &recovery, RecoveryDurationMillis: 30000,
		StaticHostsSettings: StaticHostsConfigurationSettings{Hosts: []string{"proxy.example"}},
		ProxySQLCapacitySettings: &ProxySQLCapacityConfigurationSettings{
			Metric: "frontend", FrontendMaxConnections: 200000, WriterHostgroupID: 30,
			BackendLimits: []ProxySQLBackendLimit{{Host: "/var/run/mysqld/mysqld.sock", Port: 0, MaxConnections: 1000}},
		},
	}
}

func TestCapacityDoesNotInheritMySQLProbeSettings(t *testing.T) {
	cluster := capacityClusterSettings()
	settings := MySQLConfigurationSettings{
		User: "mysql-user", Password: "mysql-password", Port: 3306,
		MetricQuery: "SELECT mysql_only", IgnoreHostsCount: 1, IgnoreHosts: []string{"proxy"},
		HttpCheckPort: 9210, CacheMillis: 20000,
		Clusters: map[string]*MySQLClusterConfigurationSettings{"capacity": cluster},
	}
	if err := settings.postReadAdjustments(); err != nil {
		t.Fatal(err)
	}
	if cluster.Port != 6032 || cluster.MetricQuery != "" || cluster.IgnoreHostsCount != 0 ||
		len(cluster.IgnoreHosts) != 0 || cluster.HttpCheckPort != -1 || cluster.CacheMillis != 0 || !cluster.FailOnNoHosts {
		t.Fatalf("capacity inherited unsafe MySQL settings: %+v", cluster)
	}
}

func TestCapacityConfigurationRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name   string
		change func(*MySQLClusterConfigurationSettings)
	}{
		{"missing user", func(c *MySQLClusterConfigurationSettings) { c.User = "" }},
		{"missing password environment", func(c *MySQLClusterConfigurationSettings) { c.Password = "${FRENO_TEST_MISSING_STATS_PASSWORD}" }},
		{"MySQL port", func(c *MySQLClusterConfigurationSettings) { c.Port = 3306 }},
		{"application port", func(c *MySQLClusterConfigurationSettings) { c.Port = 6033 }},
		{"custom query", func(c *MySQLClusterConfigurationSettings) { c.MetricQuery = "SELECT mysql_only" }},
		{"missing discovery", func(c *MySQLClusterConfigurationSettings) { c.StaticHostsSettings.Hosts = nil }},
		{"backend discovery", func(c *MySQLClusterConfigurationSettings) { c.ProxySQLSettings.Addresses = []string{"proxy:6032"} }},
		{"ignored hosts", func(c *MySQLClusterConfigurationSettings) { c.IgnoreHostsCount = 1 }},
		{"no recovery", func(c *MySQLClusterConfigurationSettings) { c.RecoveryDurationMillis = 0 }},
		{"invalid throttle", func(c *MySQLClusterConfigurationSettings) { c.ThrottleThreshold = 101 }},
		{"missing recovery threshold", func(c *MySQLClusterConfigurationSettings) { c.RecoveryThreshold = nil }},
		{"invalid metric", func(c *MySQLClusterConfigurationSettings) { c.ProxySQLCapacitySettings.Metric = "unknown" }},
		{"zero frontend capacity", func(c *MySQLClusterConfigurationSettings) { c.ProxySQLCapacitySettings.FrontendMaxConnections = 0 }},
		{"zero backend capacity", func(c *MySQLClusterConfigurationSettings) {
			c.ProxySQLCapacitySettings.BackendLimits[0].MaxConnections = 0
		}},
		{"missing backend limits", func(c *MySQLClusterConfigurationSettings) { c.ProxySQLCapacitySettings.BackendLimits = nil }},
	}
	t.Setenv("FRENO_TEST_MISSING_STATS_PASSWORD", "")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster := capacityClusterSettings()
			test.change(cluster)
			settings := MySQLConfigurationSettings{
				User: "must-not-inherit", Password: "must-not-inherit",
				Clusters: map[string]*MySQLClusterConfigurationSettings{"capacity": cluster},
			}
			if err := settings.postReadAdjustments(); err == nil || !strings.Contains(err.Error(), "ProxySQLCapacitySettings") {
				t.Fatalf("unsafe configuration accepted: %v", err)
			}
		})
	}
}
