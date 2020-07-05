package config

import "strings"

//
// ProxySQL-specific configuration
//

type ProxySQLConfigurationSettings struct {
	Addresses           []string
	User                string
	Password            string
	HostgroupID         uint
	IgnoreServerTTLSecs uint
}

func (settings *ProxySQLConfigurationSettings) IsEmpty() bool {
	if len(settings.Addresses) == 0 {
		return true
	} else {
		// address must contain host:port
		for _, addr := range settings.Addresses {
			if !strings.Contains(addr, ":") {
				return true
			}
		}
	}
	if settings.User == "" || settings.Password == "" {
		return true
	}
	if settings.HostgroupID < 1 {
		return true
	}
	return false
}
