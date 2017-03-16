package config

//
// MySQL-specific configuration
//

import (
	"os"
)

const DefaultMySQLPort = 3306

type MySQLClusterConfigurationSettings struct {
	User              string  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	Password          string  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	MetricQuery       string  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	ThrottleThreshold float64 // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	Port              int     // Specify if different than 3306 or if different than specified by MySQLConfigurationSettings

	HAProxySettings     HAProxyConfigurationSettings // If list of servers is to be acquired via HAProxy, provide this field
	StaticHostsSettings StaticHostsConfigurationSettings
}

// Hook to implement adjustments after reading each configuration file.
func (settings *MySQLClusterConfigurationSettings) postReadAdjustments() error {
	// Username & password may be given as plaintext in the config file, or can be delivered
	// via environment variables. We accept user & password in the form "${SOME_ENV_VARIABLE}"
	// in which case we get the value from this process' invoking environment.
	if submatch := envVariableRegexp.FindStringSubmatch(settings.User); len(submatch) > 1 {
		settings.User = os.Getenv(submatch[1])
	}
	if submatch := envVariableRegexp.FindStringSubmatch(settings.Password); len(submatch) > 1 {
		settings.Password = os.Getenv(submatch[1])
	}
	return nil
}

type MySQLConfigurationSettings struct {
	User              string
	Password          string
	MetricQuery       string
	ThrottleThreshold float64
	Port              int // Specify if different than 3306; applies to all clusters

	Clusters map[string](*MySQLClusterConfigurationSettings) // cluster name -> cluster config
}

// Hook to implement adjustments after reading each configuration file.
func (settings *MySQLConfigurationSettings) postReadAdjustments() error {
	if settings.Port == 0 {
		settings.Port = DefaultMySQLPort
	}
	// Username & password may be given as plaintext in the config file, or can be delivered
	// via environment variables. We accept user & password in the form "${SOME_ENV_VARIABLE}"
	// in which case we get the value from this process' invoking environment.
	if submatch := envVariableRegexp.FindStringSubmatch(settings.User); len(submatch) > 1 {
		settings.User = os.Getenv(submatch[1])
	}
	if submatch := envVariableRegexp.FindStringSubmatch(settings.Password); len(submatch) > 1 {
		settings.Password = os.Getenv(submatch[1])
	}

	for _, clusterSettings := range settings.Clusters {
		if err := clusterSettings.postReadAdjustments(); err != nil {
			return err
		}
		if clusterSettings.User == "" {
			clusterSettings.User = settings.User
		}
		if clusterSettings.Password == "" {
			clusterSettings.Password = settings.Password
		}
		if clusterSettings.MetricQuery == "" {
			clusterSettings.MetricQuery = settings.MetricQuery
		}
		if clusterSettings.ThrottleThreshold == 0 {
			clusterSettings.ThrottleThreshold = settings.ThrottleThreshold
		}
		if clusterSettings.Port == 0 {
			clusterSettings.Port = settings.Port
		}
	}
	return nil
}
