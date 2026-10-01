package config

//
// MySQL-specific configuration
//

import (
	"fmt"
	"os"
	"sort"
)

const DefaultMySQLPort = 3306

type MySQLClusterConfigurationSettings struct {
	User                   string   // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	Password               string   // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	MetricQuery            string   // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	CacheMillis            int      // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	ThrottleThreshold      float64  // override MySQLConfigurationSettings's, or leave empty to inherit those settings
	RecoveryThreshold      *float64 // optional threshold that must be met before a throttled cluster begins recovery
	RecoveryDurationMillis int      // continuous recovery duration required before writes resume
	FailOnNoHosts          bool     // reject checks when no eligible hosts remain instead of preserving legacy fail-open behavior
	RequiredClusters       []string // additional MySQL cluster metrics that must pass for this cluster to pass
	Port                   int      // Specify if different than 3306 or if different than specified by MySQLConfigurationSettings
	IgnoreHostsCount       int      // Number of hosts that can be skipped/ignored even on error or on exceeding theesholds
	IgnoreHostsThreshold   float64  // Threshold beyond which IgnoreHostsCount applies (default: 0)
	HttpCheckPort          int      // Specify if different than specified by MySQLConfigurationSettings. -1 to disable HTTP check
	HttpCheckPath          string   // Specify if different than specified by MySQLConfigurationSettings
	IgnoreHosts            []string // override MySQLConfigurationSettings's, or leave empty to inherit those settings

	HAProxySettings     HAProxyConfigurationSettings  // If list of servers is to be acquired via HAProxy, provide this field
	ProxySQLSettings    ProxySQLConfigurationSettings // If list of servers is to be acquired via ProxySQL, provide this field
	VitessSettings      VitessConfigurationSettings   // If list of servers is to be acquired via Vitess, provide this field
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
	if err := settings.HAProxySettings.postReadAdjustments(); err != nil {
		return err
	}
	return nil
}

type MySQLConfigurationSettings struct {
	User                 string
	Password             string
	MetricQuery          string
	CacheMillis          int // optional, if defined then probe result will be cached, and future probes may use cached value
	ThrottleThreshold    float64
	Port                 int      // Specify if different than 3306; applies to all clusters
	IgnoreDialTcpErrors  bool     // Skip hosts where a metric cannot be retrieved due to TCP dial errors
	IgnoreHostsCount     int      // Number of hosts that can be skipped/ignored even on error or on exceeding theesholds
	IgnoreHostsThreshold float64  // Threshold beyond which IgnoreHostsCount applies (default: 0)
	HttpCheckPort        int      // port for HTTP check. -1 to disable.
	HttpCheckPath        string   // If non-empty, requires HttpCheckPort
	IgnoreHosts          []string // If non empty, substrings to indicate hosts to be ignored/skipped
	ProxySQLAddresses    []string // A list of ProxySQL instances to query for hosts
	ProxySQLUser         string   // ProxySQL stats username
	ProxySQLPassword     string   // ProxySQL stats password
	VitessCells          []string // Name of the Vitess cells for polling tablet hosts
	Collation            string   // MySQL collation to use for stores, replaces charset if specified

	FallbackCluster string
	Clusters        map[string](*MySQLClusterConfigurationSettings) // cluster name -> cluster config
}

// Hook to implement adjustments after reading each configuration file.
func (settings *MySQLConfigurationSettings) postReadAdjustments() error {
	if settings.Port == 0 {
		settings.Port = DefaultMySQLPort
	}
	if settings.Clusters == nil {
		settings.Clusters = make(map[string]*MySQLClusterConfigurationSettings)
	}
	if settings.FallbackCluster != "" {
		if _, ok := settings.Clusters[settings.FallbackCluster]; !ok {
			return fmt.Errorf("Stores.MySQL.FallbackCluster %q does not name a configured cluster", settings.FallbackCluster)
		}
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

	for i := 0; i < len(settings.VitessCells); i++ {
		cell := settings.VitessCells[i]
		if submatch := envVariableRegexp.FindStringSubmatch(cell); len(submatch) > 1 {
			settings.VitessCells[i] = os.Getenv(submatch[1])
		}
	}

	for clusterName, clusterSettings := range settings.Clusters {
		if clusterSettings == nil {
			return fmt.Errorf("Stores.MySQL.Clusters.%s must not be null", clusterName)
		}
		if err := clusterSettings.postReadAdjustments(); err != nil {
			return err
		}
		if !clusterSettings.VitessSettings.IsEmpty() {
			if err := clusterSettings.VitessSettings.postReadAdjustments(); err != nil {
				return fmt.Errorf("Stores.MySQL.Clusters.%s.VitessSettings: %w", clusterName, err)
			}
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
		if clusterSettings.CacheMillis == 0 {
			clusterSettings.CacheMillis = settings.CacheMillis
		}
		if clusterSettings.ThrottleThreshold == 0 {
			clusterSettings.ThrottleThreshold = settings.ThrottleThreshold
		}
		if clusterSettings.Port == 0 {
			clusterSettings.Port = settings.Port
		}
		if clusterSettings.IgnoreHostsCount == 0 {
			clusterSettings.IgnoreHostsCount = settings.IgnoreHostsCount
		}
		if clusterSettings.IgnoreHostsThreshold == 0 {
			clusterSettings.IgnoreHostsThreshold = settings.IgnoreHostsThreshold
		}
		if clusterSettings.HttpCheckPort == 0 {
			clusterSettings.HttpCheckPort = settings.HttpCheckPort
		}
		if clusterSettings.HttpCheckPath == "" {
			clusterSettings.HttpCheckPath = settings.HttpCheckPath
		}
		if len(clusterSettings.IgnoreHosts) == 0 {
			clusterSettings.IgnoreHosts = settings.IgnoreHosts
		}
		if !clusterSettings.ProxySQLSettings.IsEmpty() {
			if len(clusterSettings.ProxySQLSettings.Addresses) < 1 {
				clusterSettings.ProxySQLSettings.Addresses = settings.ProxySQLAddresses
			}
			if clusterSettings.ProxySQLSettings.User == "" {
				clusterSettings.ProxySQLSettings.User = settings.ProxySQLUser
			}
			if clusterSettings.ProxySQLSettings.Password == "" {
				clusterSettings.ProxySQLSettings.Password = settings.ProxySQLPassword
			}
		}
		if !clusterSettings.VitessSettings.IsEmpty() && len(clusterSettings.VitessSettings.Cells) < 1 {
			clusterSettings.VitessSettings.Cells = settings.VitessCells
		}
		if clusterSettings.RecoveryDurationMillis < 0 {
			return fmt.Errorf("Stores.MySQL.Clusters.%s.RecoveryDurationMillis must not be negative", clusterName)
		}
		if clusterSettings.RecoveryDurationMillis > 0 {
			recoveryThreshold := clusterSettings.ThrottleThreshold
			if clusterSettings.RecoveryThreshold != nil {
				recoveryThreshold = *clusterSettings.RecoveryThreshold
			}
			if recoveryThreshold > clusterSettings.ThrottleThreshold {
				return fmt.Errorf(
					"Stores.MySQL.Clusters.%s.RecoveryThreshold must not exceed ThrottleThreshold",
					clusterName,
				)
			}
		}
	}
	return settings.validateRequiredClusters()
}

func (settings *MySQLConfigurationSettings) validateRequiredClusters() error {
	const (
		unvisited = iota
		visiting
		visited
	)
	states := make(map[string]int)

	var visit func(string) error
	visit = func(clusterName string) error {
		switch states[clusterName] {
		case visiting:
			return fmt.Errorf("Stores.MySQL.Clusters contains a RequiredClusters cycle involving %q", clusterName)
		case visited:
			return nil
		}

		states[clusterName] = visiting
		for _, requiredCluster := range settings.Clusters[clusterName].RequiredClusters {
			if _, ok := settings.Clusters[requiredCluster]; !ok {
				return fmt.Errorf(
					"Stores.MySQL.Clusters.%s.RequiredClusters references unknown cluster %q",
					clusterName,
					requiredCluster,
				)
			}
			if err := visit(requiredCluster); err != nil {
				return err
			}
		}
		states[clusterName] = visited
		return nil
	}

	clusterNames := make([]string, 0, len(settings.Clusters))
	for clusterName := range settings.Clusters {
		clusterNames = append(clusterNames, clusterName)
	}
	sort.Strings(clusterNames)
	for _, clusterName := range clusterNames {
		if states[clusterName] == unvisited {
			if err := visit(clusterName); err != nil {
				return err
			}
		}
	}
	return nil
}
