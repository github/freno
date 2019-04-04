package config

//
// HAProxy-specific configuration
//

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type HostPort struct {
	Host string
	Port int
}

func (h *HostPort) String() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

type HAProxyConfigurationSettings struct {
	Host      string
	Port      int
	Addresses string
	PoolName  string
}

func (settings *HAProxyConfigurationSettings) parseHostPort(address string) (hostPort *HostPort, err error) {
	tokens := strings.SplitN(address, ":", 2)
	if len(tokens) != 2 {
		return nil, fmt.Errorf("Cannot parse HostPort from %s. Expected format is host:port", address)
	}

	hostPort = &HostPort{Host: tokens[0]}
	if hostPort.Port, err = strconv.Atoi(tokens[1]); err != nil {
		return hostPort, fmt.Errorf("Invalid port: %s", tokens[1])
	}

	return hostPort, nil

}

func (settings *HAProxyConfigurationSettings) parseAddresses() (addresses [](*HostPort), err error) {
	tokens := strings.Split(settings.Addresses, ",")
	for _, token := range tokens {
		if token = strings.TrimSpace(token); token != "" {
			hostPort, err := settings.parseHostPort(token)
			if err != nil {
				return addresses, err
			}
			addresses = append(addresses, hostPort)
		}
	}
	return addresses, err
}

func (settings *HAProxyConfigurationSettings) GetProxyAddresses() [](*HostPort) {
	if settings.Host != "" && settings.Port > 0 {
		h := &HostPort{Host: settings.Host, Port: settings.Port}
		return [](*HostPort){h}
	}
	addresses, err := settings.parseAddresses()
	if err != nil {
		return [](*HostPort){}
	}
	return addresses
}

func (settings *HAProxyConfigurationSettings) IsEmpty() bool {
	if settings.PoolName == "" {
		return true
	}
	return len(settings.GetProxyAddresses()) == 0
}

func (settings *HAProxyConfigurationSettings) postReadAdjustments() error {
	for {
		submatch := envVariableRegexp.FindStringSubmatch(settings.Addresses)
		if len(submatch) == 0 {
			break
		}
		envVar := fmt.Sprintf("${%s}", submatch[1])
		envValue := os.Getenv(submatch[1])
		if envValue == "" {
			return fmt.Errorf("HAProxySettings: unknown environment variable %s", envVar)
		}
		settings.Addresses = strings.Replace(settings.Addresses, envVar, envValue, -1)
	}
	fmt.Printf("=========== Addresses: %s\n", settings.Addresses)

	return nil
}
