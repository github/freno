/*
   Copyright 2015 Shlomi Noach, courtesy Booking.com
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultMySQLPort = 3306
)

// InstanceKey is an instance indicator, identifued by hostname and port
type InstanceKey struct {
	Hostname string
	Port     int
}

// NewRawInstanceKey will parse an InstanceKey from a string representation such as 127.0.0.1:3306
// It expects such format and returns with error if input differs in format
func newRawInstanceKey(hostPort string) (*InstanceKey, error) {
	tokens := strings.SplitN(hostPort, ":", 2)
	if len(tokens) != 2 {
		return nil, fmt.Errorf("Cannot parse InstanceKey from %s. Expected format is host:port", hostPort)
	}
	instanceKey := &InstanceKey{Hostname: tokens[0]}
	var err error
	if instanceKey.Port, err = strconv.Atoi(tokens[1]); err != nil {
		return instanceKey, fmt.Errorf("Invalid port: %s", tokens[1])
	}

	return instanceKey, nil
}

// ParseInstanceKey will parse an InstanceKey from a string representation such as 127.0.0.1:3306 or some.hostname
// `defaultPort` is used if `hostPort` does not include a port.
func ParseInstanceKey(hostPort string, defaultPort int) (*InstanceKey, error) {
	if !strings.Contains(hostPort, ":") {
		return &InstanceKey{Hostname: hostPort, Port: defaultPort}, nil
	}
	return newRawInstanceKey(hostPort)
}

// Equals tests equality between this key and another key
func (this *InstanceKey) Equals(other *InstanceKey) bool {
	if other == nil {
		return false
	}
	return this.Hostname == other.Hostname && this.Port == other.Port
}

// SmallerThan returns true if this key is dictionary-smaller than another.
// This is used for consistent sorting/ordering; there's nothing magical about it.
func (this *InstanceKey) SmallerThan(other *InstanceKey) bool {
	if this.Hostname < other.Hostname {
		return true
	}
	if this.Hostname == other.Hostname && this.Port < other.Port {
		return true
	}
	return false
}

// IsValid uses simple heuristics to see whether this key represents an actual instance
func (this *InstanceKey) IsValid() bool {
	if this.Hostname == "_" {
		return false
	}
	return len(this.Hostname) > 0 && this.Port > 0
}

// StringCode returns an official string representation of this key
func (this *InstanceKey) StringCode() string {
	return fmt.Sprintf("%s:%d", this.Hostname, this.Port)
}

// DisplayString returns a user-friendly string representation of this key
func (this *InstanceKey) DisplayString() string {
	return this.StringCode()
}

// String returns a user-friendly string representation of this key
func (this InstanceKey) String() string {
	return this.StringCode()
}
