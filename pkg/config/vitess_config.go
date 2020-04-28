package config

//
// HAProxy-specific configuration
//

type VitessConfigurationSettings struct {
	API         string
	Keyspace    string
	Shard       string
	TimeoutSecs float64
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
