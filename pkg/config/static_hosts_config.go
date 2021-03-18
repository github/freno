package config

//Static hosts configuration
//

type StaticHostsConfigurationSettings struct {
	Hosts []string // a host can be "hostname" or "hostname:port"
}

func (settings *StaticHostsConfigurationSettings) IsEmpty() bool {
	return len(settings.Hosts) == 0
}
