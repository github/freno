package config

import (
	"encoding/json"
	"os"

	"github.com/outbrain/golib/log"
)

var instance *Configuration = newConfiguration()

// Instance returns the global instance of Configuration
func Instance() *Configuration {
	return instance
}

// Params returns the settings of the global instance of Configuration
func Settings() *configurationSettings {
	return Instance().settings
}

type Configuration struct {
	readFileNames []string
	settings      *configurationSettings
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
			if err == nil {
				decoder := json.NewDecoder(file)
				err := decoder.Decode(settings)
				if err == nil {
					log.Infof("Read config: %s", fileName)
				} else {
					log.Fatal("Cannot read config file:", fileName, err)
					return err
				}
			}
		}
	}

	if err := settings.postReadAdjustments(); err != nil {
		log.Fatale(err)
		return err
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
type configurationSettings struct {
	ListenPort int
	// Debug                                        bool   // set debug mode (similar to --debug option)
	// ListenSocket                                 string // Where freno HTTP should listen for unix socket (default: empty; when given, TCP is disabled)
	// AnExampleSliceOfStrings                    []string // Add a comment here
	// AnExampleMapOfStringsToStrings    map[string]string // Add a comment here
}

func newConfigurationSettings() *configurationSettings {
	return &configurationSettings{
		ListenPort: 8087,
		//Debug:                                        false,
		//ListenSocket:                                 "",
		//AnExampleListOfStrings:                       []string{"*"},
		//AnExampleMapOfStringsToStrings:               make(map[string]string),
	}
}

// Hook to implement adjustments after reading each configuration file.
func (this *configurationSettings) postReadAdjustments() error {
	return nil
}
