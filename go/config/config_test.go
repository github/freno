package config

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

func TestReadSingleFile(t *testing.T) {
	var config = newConfiguration()
	newPort := 65534

	config.parameters.ListenPort = newPort
	dump("/tmp/TestReadSingleFileFixture.json", config.parameters)

	config = newConfiguration()
	config.Read("/tmp/TestReadSingleFileFixture.json")
	if config.parameters.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.parameters.ListenPort, newPort)
	}
}

func TestReadMultipleFiles(t *testing.T) {
	var config = newConfiguration()
	newPort := 65534
	newerPort := 65535

	config.parameters.ListenPort = newPort
	dump("/tmp/TestReadMultipleFiles1.json", config.parameters)

	config.parameters.ListenPort = newerPort
	dump("/tmp/TestReadMultipleFiles2.json", config.parameters)

	// Value is overwritten in order
	config = newConfiguration()
	config.Read("/tmp/TestReadMultipleFiles1.json", "/tmp/TestReadMultipleFiles2.json")
	if config.parameters.ListenPort != newerPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.parameters.ListenPort, newerPort)
	}

	// Value is overwritten in order
	config = newConfiguration()
	config.Read("/tmp/TestReadMultipleFiles2.json", "/tmp/TestReadMultipleFiles1.json")
	if config.parameters.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.parameters.ListenPort, newPort)
	}
}

func TestReaload(t *testing.T) {
	var config = newConfiguration()
	newPort := 65534
	temporaryChangedPort := 8080

	config.parameters.ListenPort = newPort
	dump("/tmp/TestReloadFixture.json", config.parameters)

	config = newConfiguration()
	config.Read("/tmp/TestReadSingleFileFixture.json")
	if config.parameters.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reading it from configuration", config.parameters.ListenPort, newPort)
	}

	config.parameters.ListenPort = temporaryChangedPort
	config.Reload()
	if config.parameters.ListenPort != newPort {
		t.Errorf("Expected ListenPort %d to be %d after reloading the configuration", config.parameters.ListenPort, newPort)
	}
}

func dump(path string, contents *configurationParameters) error {
	json, _ := json.Marshal(contents)
	err := ioutil.WriteFile(path, json, 0644)
	return err
}
