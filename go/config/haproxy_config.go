package config

//
// HAProxy-specific configuration
//

type HAProxyConfigurationSettings struct {
	Url      string
	PoolName string
}

func (settings *HAProxyConfigurationSettings) IsEmpty() bool {
	if settings.Url == "" {
		return true
	}
	if settings.PoolName == "" {
		return true
	}
	return false
}
