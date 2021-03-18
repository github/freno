package config

//
// HAProxy-specific configuration
//

import (
	"fmt"
	"net/url"
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

func (h *HostPort) URL() *url.URL {
	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", h.Host, h.Port))
	return u
}

func ParseHostPort(address string) (hostPort *HostPort, err error) {
	if !strings.Contains(address, ":") {
		if address == "" {
			return &HostPort{address, 80}, fmt.Errorf("Invalid host address: %s", address)
		}

		return &HostPort{Host: address, Port: 80}, nil
	}

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

type HAProxyConfigurationSettings struct {
	Host      string
	Port      int
	Addresses string
	PoolName  string
}

func parseAddress(address string) (u *url.URL, err error) {
	if hostPort, err := ParseHostPort(address); err == nil {
		// covers the case for e.g. "my.host.name:1234", which has no scheme
		return hostPort.URL(), nil
	}
	u, err = url.Parse(address)

	if err != nil {
		return u, err
	}
	if _, err := ParseHostPort(u.Host); err != nil {
		return u, err
	}
	return u, nil
}

func (settings *HAProxyConfigurationSettings) parseAddresses() (addresses [](*url.URL), err error) {
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

func (settings *HAProxyConfigurationSettings) GetProxyAddresses() (addresses [](*url.URL), err error) {
	if settings.Host != "" && settings.Port > 0 {
		u := (&HostPort{Host: settings.Host, Port: settings.Port}).URL()
		return [](*url.URL){u}, nil
	}
	return settings.parseAddresses()
}

func (settings *HAProxyConfigurationSettings) IsEmpty() bool {
	if settings.PoolName == "" {
		return true
	}
	addresses, _ := settings.GetProxyAddresses()
	return len(addresses) == 0
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

	return nil
}
