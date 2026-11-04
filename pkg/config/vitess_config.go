package config

import (
	"fmt"
	"net"
	"strconv"
)

type VitessConfigurationSettings struct {
	API            string
	GRPCAddress    string
	GRPCInsecure   bool
	GRPCCAFile     string
	GRPCServerName string
	Cells          []string
	Keyspace       string
	Shard          string
	TimeoutSecs    uint
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

// Endpoint returns the explicitly selected discovery endpoint.
func (settings VitessConfigurationSettings) Endpoint() string {
	if settings.GRPCAddress != "" {
		return settings.GRPCAddress
	}
	return settings.API
}

// Validate rejects incomplete gRPC configuration without changing legacy HTTP configuration.
func (settings VitessConfigurationSettings) Validate() error {
	if settings.GRPCAddress == "" {
		if settings.GRPCInsecure || settings.GRPCCAFile != "" || settings.GRPCServerName != "" {
			return fmt.Errorf("VitessSettings.GRPCAddress is required with gRPC transport settings")
		}
		return nil
	}
	host, port, err := net.SplitHostPort(settings.GRPCAddress)
	if err != nil || host == "" {
		return fmt.Errorf("VitessSettings.GRPCAddress must be an explicit host:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("VitessSettings.GRPCAddress must include a valid port")
	}
	if settings.Keyspace == "" {
		return fmt.Errorf("VitessSettings.Keyspace is required with GRPCAddress")
	}
	if settings.GRPCInsecure && (settings.GRPCCAFile != "" || settings.GRPCServerName != "") {
		return fmt.Errorf("VitessSettings.GRPCInsecure cannot be combined with TLS settings")
	}
	return nil
}
