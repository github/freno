package config

//
// ProxySQL-specific configuration
//

type ProxySQLConfigurationSettings struct {
	Addresses        []string
	User             string
	Password         string
	HostgroupComment string
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
