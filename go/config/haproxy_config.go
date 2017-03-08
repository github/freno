package config

//
// HAProxy-specific configuration
//

type HAProxyConfigurationSettings struct {
	Host     string
	Port     int
	PoolName string
}

func (settings *HAProxyConfigurationSettings) IsEmpty() bool {
	if settings.Host == "" {
		return true
	}
	if settings.Port == 0 {
		return true
	}
	if settings.PoolName == "" {
		return true
	}
	return false
}
