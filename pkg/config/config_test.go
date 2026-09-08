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

func dump(path string, contents *ConfigurationSettings) error {
	json, _ := json.Marshal(contents)
	err := ioutil.WriteFile(path, json, 0644)
	return err
}
