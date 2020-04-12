package vitess

//
// HAProxy-specific configuration
//

type ConfigurationSettings struct {
	API      string
	Keyspace string
	Shard    string
}

func (settings *ConfigurationSettings) IsEmpty() bool {
	if settings.API == "" {
		return true
	}
	if settings.Keyspace == "" {
		return true
	}
	return false
}
