package config

//
// ProxySQL-specific configuration
//

type ProxySQLConfigurationSettings struct {
	Addresses           []string
	User                string
	Password            string
	HostgroupComment    string
	HostgroupID         uint
	IgnoreServerTTLSecs uint
}

func (settings *ProxySQLConfigurationSettings) IsEmpty() bool {
	if len(settings.Addresses) == 0 {
		return true
	}
	if settings.User == "" || settings.Password == "" {
		return true
	}
	if settings.HostgroupComment == "" && settings.HostgroupID > 0 {
		return true
	}
	return false
}
