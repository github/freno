package config

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/outbrain/golib/log"
)

func createConfiguration() *Configuration {
	config := newConfiguration()
	config.settings.RaftDataDir = "/tmp"
	return config
}

func TestReadWrongFile(t *testing.T) {
	config := Instance()
	ioutil.WriteFile("/tmp/CorruptedFixture.json", []byte("}---{"), 0644)
	err := config.Read("/tmp/CorruptedFixture.json")
	if err == nil {
		log.Infof("Config failed")
		t.Errorf("Should have errored")
	}
}

func TestReadSingleFile(t *testing.T) {
	var config = createConfiguration()
	newPort := 65534

	config.settings.ListenPort = newPort
	dump("/tmp/TestReadSingleFileFixture.json", config.settings)

	config = createConfiguration()
	config.Read("/tmp/TestReadSingleFileFixture.json")
	if config.settings.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newPort)
	}
}

func TestReadMultipleFiles(t *testing.T) {
	var config = createConfiguration()
	newPort := 65534
	newerPort := 65535

	config.settings.ListenPort = newPort
	dump("/tmp/TestReadMultipleFiles1.json", config.settings)

	config.settings.ListenPort = newerPort
	dump("/tmp/TestReadMultipleFiles2.json", config.settings)

	// Value is overwritten in order
	config = createConfiguration()
	config.Read("/tmp/TestReadMultipleFiles1.json", "/tmp/TestReadMultipleFiles2.json")
	if config.settings.ListenPort != newerPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newerPort)
	}

	// Value is overwritten in order
	config = createConfiguration()
	config.Read("/tmp/TestReadMultipleFiles2.json", "/tmp/TestReadMultipleFiles1.json")
	if config.settings.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newPort)
	}
}

func TestReload(t *testing.T) {
	var config = createConfiguration()
	newPort := 65534
	temporaryChangedPort := 8080

	config.settings.ListenPort = newPort
	dump("/tmp/TestReloadFixture.json", config.settings)

	config = createConfiguration()
	config.Read("/tmp/TestReadSingleFileFixture.json")
	if config.settings.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newPort)
	}

	config.settings.ListenPort = temporaryChangedPort
	config.Reload()
	if config.settings.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reloading the configuration", config.settings.ListenPort, newPort)
	}
}

func TestMySQLFallbackCluster(t *testing.T) {
	tests := []struct {
		name            string
		fallbackCluster string
		clusters        map[string]*MySQLClusterConfigurationSettings
		wantErr         string
	}{
		{name: "omitted"},
		{name: "empty", clusters: map[string]*MySQLClusterConfigurationSettings{"primary": {}}},
		{name: "configured", fallbackCluster: "primary", clusters: map[string]*MySQLClusterConfigurationSettings{"primary": {}}},
		{name: "missing", fallbackCluster: "missing", clusters: map[string]*MySQLClusterConfigurationSettings{"primary": {}}, wantErr: `Stores.MySQL.FallbackCluster "missing" does not name a configured cluster`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := MySQLConfigurationSettings{FallbackCluster: test.fallbackCluster, Clusters: test.clusters}
			err := settings.postReadAdjustments()
			if test.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != "" && (err == nil || err.Error() != test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestMySQLRequiredClusters(t *testing.T) {
	tests := []struct {
		name     string
		clusters map[string]*MySQLClusterConfigurationSettings
		wantErr  string
	}{
		{
			name: "valid dependencies",
			clusters: map[string]*MySQLClusterConfigurationSettings{
				"replicas": {RequiredClusters: []string{"primary", "proxysql"}},
				"primary":  {},
				"proxysql": {},
			},
		},
		{
			name: "unknown dependency",
			clusters: map[string]*MySQLClusterConfigurationSettings{
				"replicas": {RequiredClusters: []string{"primary"}},
			},
			wantErr: `Stores.MySQL.Clusters.replicas.RequiredClusters references unknown cluster "primary"`,
		},
		{
			name: "dependency cycle",
			clusters: map[string]*MySQLClusterConfigurationSettings{
				"replicas": {RequiredClusters: []string{"primary"}},
				"primary":  {RequiredClusters: []string{"replicas"}},
			},
			wantErr: `Stores.MySQL.Clusters contains a RequiredClusters cycle involving "primary"`,
		},
		{
			name: "null cluster",
			clusters: map[string]*MySQLClusterConfigurationSettings{
				"replicas": nil,
			},
			wantErr: `Stores.MySQL.Clusters.replicas must not be null`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := MySQLConfigurationSettings{Clusters: test.clusters}
			err := settings.postReadAdjustments()
			if test.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != "" && (err == nil || err.Error() != test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestMySQLRecoveryConfiguration(t *testing.T) {
	recoveryThreshold := 5.0
	tooHighRecoveryThreshold := 11.0
	tests := []struct {
		name            string
		clusterSettings *MySQLClusterConfigurationSettings
		wantErr         string
	}{
		{name: "disabled", clusterSettings: &MySQLClusterConfigurationSettings{ThrottleThreshold: 10}},
		{name: "valid", clusterSettings: &MySQLClusterConfigurationSettings{ThrottleThreshold: 10, RecoveryThreshold: &recoveryThreshold, RecoveryDurationMillis: 5000}},
		{name: "default recovery threshold", clusterSettings: &MySQLClusterConfigurationSettings{ThrottleThreshold: 10, RecoveryDurationMillis: 5000}},
		{
			name:            "negative duration",
			clusterSettings: &MySQLClusterConfigurationSettings{ThrottleThreshold: 10, RecoveryDurationMillis: -1},
			wantErr:         "Stores.MySQL.Clusters.primary.RecoveryDurationMillis must not be negative",
		},
		{
			name:            "recovery threshold above throttle threshold",
			clusterSettings: &MySQLClusterConfigurationSettings{ThrottleThreshold: 10, RecoveryThreshold: &tooHighRecoveryThreshold, RecoveryDurationMillis: 5000},
			wantErr:         "Stores.MySQL.Clusters.primary.RecoveryThreshold must not exceed ThrottleThreshold",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := MySQLConfigurationSettings{
				Clusters: map[string]*MySQLClusterConfigurationSettings{"primary": test.clusterSettings},
			}
			err := settings.postReadAdjustments()
			if test.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != "" && (err == nil || err.Error() != test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestVitessTabletTypeConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		tabletType string
		want       string
		wantErr    string
	}{
		{name: "default replica", want: "REPLICA"},
		{name: "replica", tabletType: "replica", want: "REPLICA"},
		{name: "master", tabletType: "master", want: "MASTER"},
		{name: "primary alias", tabletType: "primary", want: "MASTER"},
		{name: "invalid", tabletType: "rdonly", wantErr: `Stores.MySQL.Clusters.vitess.VitessSettings: unsupported Vitess tablet type "rdonly"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := MySQLConfigurationSettings{
				Clusters: map[string]*MySQLClusterConfigurationSettings{
					"vitess": {
						VitessSettings: VitessConfigurationSettings{
							API:        "https://vtctld.example.com/api",
							Keyspace:   "test",
							TabletType: test.tabletType,
						},
					},
				},
			}
			err := settings.postReadAdjustments()
			if test.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if got := settings.Clusters["vitess"].VitessSettings.TabletType; got != test.want {
				t.Fatalf("TabletType = %q, want %q", got, test.want)
			}
		})
	}
}

func dump(path string, contents *ConfigurationSettings) error {
	json, _ := json.Marshal(contents)
	err := ioutil.WriteFile(path, json, 0644)
	return err
}
