package config

import "fmt"

//
// ProxySQL-specific configuration
//

const proxySQLDefaultDatabase = "stats"

type ProxySQLConfigurationSettings struct {
	Addresses           []string
	User                string
	Password            string
	HostgroupID         uint
	IgnoreServerTTLSecs uint
}

func (settings ProxySQLConfigurationSettings) AddressToDSN(address string) string {
	return fmt.Sprintf("mysql://%s:*****@%s/%s", settings.User, address, proxySQLDefaultDatabase)
}

func (settings *ProxySQLConfigurationSettings) IsEmpty() bool {
	if len(settings.Addresses) == 0 {
		return true
	}
	if settings.User == "" || settings.Password == "" {
		return true
	}
	if settings.HostgroupID < 1 {
		return true
	}
	return false
}
