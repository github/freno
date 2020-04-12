package haproxy

//
// HAProxy-specific configuration
//

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/github/freno/pkg/config/util"
)

func ParseHostPort(address string) (hostPort *util.HostPort, err error) {
	if !strings.Contains(address, ":") {
		if address == "" {
			return &util.HostPort{address, 80}, fmt.Errorf("Invalid host address: %s", address)
		}

		return &util.HostPort{Host: address, Port: 80}, nil
	}

	tokens := strings.SplitN(address, ":", 2)
	if len(tokens) != 2 {
		return nil, fmt.Errorf("Cannot parse HostPort from %s. Expected format is host:port", address)
	}

	hostPort = &util.HostPort{Host: tokens[0]}
	if hostPort.Port, err = strconv.Atoi(tokens[1]); err != nil {
		return hostPort, fmt.Errorf("Invalid port: %s", tokens[1])
	}

	return hostPort, nil
}

type ConfigurationSettings struct {
	Host      string
	Port      int
	Addresses string
	PoolName  string
}

func parseAddress(address string) (u *util.HostPort, err error) {
	if hostPort, err := ParseHostPort(address); err == nil {
		// covers the case for e.g. "my.host.name:1234", which has no scheme
		return hostPort, nil
	}
	return ParseHostPort(u.Host)
}

func (settings *ConfigurationSettings) parseAddresses() (addresses [](*util.HostPort), err error) {
	tokens := strings.Split(settings.Addresses, ",")
	for _, token := range tokens {
		if token = strings.TrimSpace(token); token != "" {
			u, err := parseAddress(token)
			if err != nil {
				return addresses, err
			}
			addresses = append(addresses, u)
		}
	}
	return addresses, err
}

func (settings *ConfigurationSettings) GetProxyAddresses() (addresses [](*util.HostPort), err error) {
	if settings.Host != "" && settings.Port > 0 {
		hp := &util.HostPort{Host: settings.Host, Port: settings.Port}
		return [](*util.HostPort){hp}, nil
	}
	return settings.parseAddresses()
}

func (settings *ConfigurationSettings) IsEmpty() bool {
	if settings.PoolName == "" {
		return true
	}
	addresses, _ := settings.GetProxyAddresses()
	return len(addresses) == 0
}

func (settings *ConfigurationSettings) PostReadAdjustments() error {
	for {
		submatch := util.EnvVariableRegexp.FindStringSubmatch(settings.Addresses)
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

	return nil
}
