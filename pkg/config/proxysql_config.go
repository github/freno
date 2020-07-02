package config

//
// ProxySQL-specific configuration
//

import (
	"sort"
	"strings"
)

type ProxySQLConfigurationSettings struct {
	Addresses        []string
	User             string
	Password         string
	HostgroupComment string
}

func (settings *ProxySQLConfigurationSettings) URL() string {
	addrs := settings.Addresses
	sort.Strings(addrs)
	return strings.Join(addrs, ",")
}

func (settings *ProxySQLConfigurationSettings) IsEmpty() bool {
	if len(settings.Addresses) == 0 {
		return true
	}
	if settings.User == "" || settings.Password == "" || settings.HostgroupComment == "" {
		return true
	}
	return false
}
