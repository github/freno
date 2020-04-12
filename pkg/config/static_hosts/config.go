package static_hosts

//Static hosts configuration
//

type ConfigurationSettings struct {
	Hosts []string // a host can be "hostname" or "hostname:port"
}

func (settings *ConfigurationSettings) IsEmpty() bool {
	return len(settings.Hosts) == 0
}
