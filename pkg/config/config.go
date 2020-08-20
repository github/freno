package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/outbrain/golib/log"
)

var (
	envVariableRegexp = regexp.MustCompile("[$][{](.*?)[}]")
)

var instance = newConfiguration()

// Instance returns the global instance of Configuration
func Instance() *Configuration {
	return instance
}

// Settings returns the settings of the global instance of Configuration
func Settings() *ConfigurationSettings {
	return Instance().settings
}

// Reset sets the initial state of the configuration instance
func Reset() {
	instance = newConfiguration()
}

// Configuration struct stores the readFileNames and points to the settings
// which are the configuration parameters used in the application.
// see ConfigurationSettings for the available settings.
// Read file names are also stored to allow configuration reloading.
type Configuration struct {
	readFileNames []string
	settings      *ConfigurationSettings
}

func newConfiguration() *Configuration {
	return &Configuration{
		settings: newConfigurationSettings(),
	}
}

// Read reads configuration from all given files, in order of input.
// Each file can override the properties of the previous files
// Initially, the settings are the defult ones defined by newConfigurationSettings
func (config *Configuration) Read(fileNames ...string) error {
	settings := newConfigurationSettings()

	for _, fileName := range fileNames {
		if _, err := os.Stat(fileName); err == nil {
			file, err := os.Open(fileName)

			if err != nil {
				return log.Errorf("Cannot read config file %s, error was: %s", fileName, err)
			}

			defer file.Close()

			decoder := json.NewDecoder(file)
			err = decoder.Decode(settings)

			if err == nil {
				log.Infof("Config read from %s", fileName)
			} else {
				return fmt.Errorf("Cannot read config file %s, error was: %s", fileName, err)
			}
		}
	}

	if err := settings.postReadAdjustments(); err != nil {
		return log.Errore(err)
	}

	config.readFileNames = fileNames
	config.settings = settings
	return nil
}

// Reload re-reads configuration from last used files
func (config *Configuration) Reload() error {
	return config.Read(config.readFileNames...)
}

// ConfigurationSettings models a set of configurable values, that can be
// provided by the user via one or several JSON formatted files.
//
// Some of the settinges have reasonable default values, and some other
// (like database credentials) are strictly expected from user.
type ConfigurationSettings struct {
	ListenPort           int
	DataCenter           string
	Environment          string
	Domain               string
	ShareDomain          string
	RaftBind             string
	RaftDataDir          string
	DefaultRaftPort      int      // if a RaftNodes entry does not specify port, use this one
	RaftNodes            []string // Raft nodes to make initial connection with
	BackendMySQLHost     string
	BackendMySQLPort     int
	BackendMySQLSchema   string
	BackendMySQLUser     string
	BackendMySQLPassword string
	MemcacheServers      []string // if given, freno will report to aggregated values to given memcache
	MemcachePath         string   // use as prefix to metric path in memcache key, e.g. if `MemcachePath` is "myprefix" the key would be "myprefix/mysql/maincluster". Default: "freno"
	EnableProfiling      bool     // enable pprof profiling http api
	Stores               StoresSettings
}

func newConfigurationSettings() *ConfigurationSettings {
	return &ConfigurationSettings{
		ListenPort:         8087,
		RaftBind:           "127.0.0.1:10008",
		RaftDataDir:        "",
		DefaultRaftPort:    0,
		RaftNodes:          []string{},
		BackendMySQLHost:   "",
		BackendMySQLSchema: "",
		BackendMySQLPort:   3306,
		MemcacheServers:    []string{},
		MemcachePath:       "freno",
		//Debug:                                        false,
		//ListenSocket:                                 "",
		//AnExampleListOfStrings:                       []string{"*"},
		//AnExampleMapOfStringsToStrings:               make(map[string]string),
	}
}

// Hook to implement adjustments after reading each configuration file.
func (settings *ConfigurationSettings) postReadAdjustments() error {
	if submatch := envVariableRegexp.FindStringSubmatch(settings.BackendMySQLHost); len(submatch) > 1 {
		settings.BackendMySQLHost = os.Getenv(submatch[1])
	}
	if submatch := envVariableRegexp.FindStringSubmatch(settings.BackendMySQLSchema); len(submatch) > 1 {
		settings.BackendMySQLSchema = os.Getenv(submatch[1])
	}
	if submatch := envVariableRegexp.FindStringSubmatch(settings.BackendMySQLUser); len(submatch) > 1 {
		settings.BackendMySQLUser = os.Getenv(submatch[1])
	}
	if submatch := envVariableRegexp.FindStringSubmatch(settings.BackendMySQLPassword); len(submatch) > 1 {
		settings.BackendMySQLPassword = os.Getenv(submatch[1])
	}
	if settings.RaftDataDir == "" && settings.BackendMySQLHost == "" {
		return fmt.Errorf("Either RaftDataDir or BackendMySQLHost must be set")
	}
	if settings.BackendMySQLHost != "" {
		if settings.BackendMySQLSchema == "" {
			return fmt.Errorf("BackendMySQLSchema must be set when BackendMySQLHost is specified")
		}
	}
	if err := settings.Stores.postReadAdjustments(); err != nil {
		return err
	}
	return nil
}
