package config

//
// HAProxy-specific configuration
//

type VitessConfigurationSettings struct {
	API      string
	Keyspace string
	Shard    string
}

func (settings *VitessConfigurationSettings) IsEmpty() bool {
	if settings.API == "" {
		return true
	}
	if settings.Keyspace == "" {
		return true
	}
	return false
}
