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

// Params returns the parameters of the global instance of Configuration
func Parameters() *configurationParameters {
	return Instance().parameters
}

type Configuration struct {
	readFileNames []string
	parameters    *configurationParameters
}

func newConfiguration() *Configuration {
	return &Configuration{
		parameters: newConfigurationParameters(),
	}
}

// Read reads configuration from all given files, in order of input.
// Each file can override the properties of the previous files
// Initially, the parameters are the defult ones defined by newConfigurationParameters
func (config *Configuration) Read(fileNames ...string) error {
	parameters := newConfigurationParameters()

	for _, fileName := range fileNames {
		if _, err := os.Stat(fileName); err == nil {
			file, err := os.Open(fileName)
			if err == nil {
				decoder := json.NewDecoder(file)
				err := decoder.Decode(parameters)
				if err == nil {
					log.Infof("Read config: %s", fileName)
				} else {
					log.Fatal("Cannot read config file:", fileName, err)
					return err
				}
			}
		}
	}

	if err := parameters.postReadAdjustments(); err != nil {
		log.Fatale(err)
		return err
	}

	config.readFileNames = fileNames
	config.parameters = parameters
	return nil
}

// Reload re-reads configuration from last used files
func (config *Configuration) Reload() error {
	return config.Read(config.readFileNames...)
}

// ConfigurationParameters models a set of configurable values, that can be
// provided by the user via one or several JSON formatted files.
//
// Some of the parameteres have reasonable default values, and some other
// (like database credentials) are strictly expected from user.
type configurationParameters struct {
	ListenPort int
	// Debug                                        bool   // set debug mode (similar to --debug option)
	// ListenSocket                                 string // Where freno HTTP should listen for unix socket (default: empty; when given, TCP is disabled)
	// AnExampleSliceOfStrings                    []string // Add a comment here
	// AnExampleMapOfStringsToStrings    map[string]string // Add a comment here
}

func newConfigurationParameters() *configurationParameters {
	return &configurationParameters{
		ListenPort: 8087,
		//Debug:                                        false,
		//ListenSocket:                                 "",
		//AnExampleListOfStrings:                       []string{"*"},
		//AnExampleMapOfStringsToStrings:               make(map[string]string),
	}
}

// Hook to implement adjustments after reading each configuration file.
func (this *configurationParameters) postReadAdjustments() error {
	return nil
}
