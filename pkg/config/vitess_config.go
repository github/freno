package config

//
// HAProxy-specific configuration
//

type VitessConfigurationSettings struct {
	API          string
	GRPCAddress  string // vtctld gRPC host:port; if set, it is used instead of API
	GRPCInsecure bool   // must be true with GRPCAddress, as only plaintext gRPC is supported
	Cells        []string
	Keyspace     string
	Shard        string
	TimeoutSecs  uint
}

func (settings *VitessConfigurationSettings) IsEmpty() bool {
	if settings.API == "" && settings.GRPCAddress == "" {
		return true
	}
	if settings.Keyspace == "" {
		return true
	}
	return false
}
