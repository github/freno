package config

//
// MySQL-specific configuration
//

const DefaultMySQLPort = 3306

type MySQLClusterConfigurationSettings struct {
	Username          string  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	Password          string  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	MetricQuery       string  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	ThrottleThreshold float64 // override MySQLConfigurationSettings's, or leave empty to inherit those settings

	HAProxySettings HAProxyConfigurationSettings // If list of servers is to be acquired via HAProxy, provide this field
}

type MySQLConfigurationSettings struct {
	Username          string
	Password          string
	MetricQuery       string
	ThrottleThreshold float64

	Clusters map[string]MySQLClusterConfigurationSettings // cluster name -> cluster config
}
