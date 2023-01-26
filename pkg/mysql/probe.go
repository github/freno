/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"net"

	"github.com/github/freno/pkg/config"
)

const maxPoolConnections = 3
const maxIdleConnections = 3
const timeoutMillis = 1000

// Probe is the minimal configuration required to connect to a MySQL server
type Probe struct {
	Key                 InstanceKey
	User                string
	Password            string
	MetricQuery         string
	CacheMillis         int
	QueryInProgress     int64
	HttpCheckPort       int
	HttpCheckPath       string
	HttpCheckInProgress int64
}

type Probes map[InstanceKey](*Probe)

type ClusterProbes struct {
	ClusterName          string
	IgnoreHostsCount     int
	IgnoreHostsThreshold float64
	InstanceProbes       *Probes
}

func NewProbes() *Probes {
	return &Probes{}
}

func NewProbe() *Probe {
	config := &Probe{
		Key: InstanceKey{},
	}
	return config
}

// DuplicateCredentials creates a new connection config with given key and with same credentials as this config
func (probe *Probe) DuplicateCredentials(key InstanceKey) *Probe {
	config := &Probe{
		Key:      key,
		User:     probe.User,
		Password: probe.Password,
	}
	return config
}

func (probe *Probe) Duplicate() *Probe {
	return probe.DuplicateCredentials(probe.Key)
}

func (probe *Probe) String() string {
	return fmt.Sprintf("%s, user=%s", probe.Key.DisplayString(), probe.User)
}

func (probe *Probe) Equals(other *Probe) bool {
	return probe.Key.Equals(&other.Key)
}

func (probe *Probe) GetDBUri(databaseName string) string {
	hostname := probe.Key.Hostname
	var ip = net.ParseIP(hostname)
	if (ip != nil) && (ip.To4() == nil) {
		// Wrap IPv6 literals in square brackets
		hostname = fmt.Sprintf("[%s]", hostname)
	}
	dsnCharsetCollation := "charset=utf8mb4,utf8,latin1"
	if config.Settings().Stores.MySQL.Collation != "" {
		// Set collation instead of charset, if Stores.MySQL.Collation is specified
		dsnCharsetCollation = fmt.Sprintf("collation=%s", config.Settings().Stores.MySQL.Collation)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&%s&timeout=%dms",
		probe.User,
		probe.Password,
		hostname,
		probe.Key.Port,
		databaseName,
		dsnCharsetCollation,
		timeoutMillis,
	)
}
