package config

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

func TestReadSingleFile(t *testing.T) {
	var config = newConfiguration()
	newPort := 65534

	config.settings.ListenPort = newPort
	dump("/tmp/TestReadSingleFileFixture.json", config.settings)

	config = newConfiguration()
	config.Read("/tmp/TestReadSingleFileFixture.json")
	if config.settings.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newPort)
	}
}

func TestReadMultipleFiles(t *testing.T) {
	var config = newConfiguration()
	newPort := 65534
	newerPort := 65535

	config.settings.ListenPort = newPort
	dump("/tmp/TestReadMultipleFiles1.json", config.settings)

	config.settings.ListenPort = newerPort
	dump("/tmp/TestReadMultipleFiles2.json", config.settings)

	// Value is overwritten in order
	config = newConfiguration()
	config.Read("/tmp/TestReadMultipleFiles1.json", "/tmp/TestReadMultipleFiles2.json")
	if config.settings.ListenPort != newerPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newerPort)
	}

	// Value is overwritten in order
	config = newConfiguration()
	config.Read("/tmp/TestReadMultipleFiles2.json", "/tmp/TestReadMultipleFiles1.json")
	if config.settings.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.settings.ListenPort, newPort)
	}
}

func TestReaload(t *testing.T) {
	var config = newConfiguration()
	newPort := 65534
	temporaryChangedPort := 8080

	config.settings.ListenPort = newPort
	dump("/tmp/TestReloadFixture.json", config.settings)

	config = newConfiguration()
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

func dump(path string, contents *ConfigurationSettings) error {
	json, _ := json.Marshal(contents)
	err := ioutil.WriteFile(path, json, 0644)
	return err
}
