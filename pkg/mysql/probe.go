/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"net"
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
func (this *Probe) DuplicateCredentials(key InstanceKey) *Probe {
	config := &Probe{
		Key:      key,
		User:     this.User,
		Password: this.Password,
	}
	return config
}

func (this *Probe) Duplicate() *Probe {
	return this.DuplicateCredentials(this.Key)
}

func (this *Probe) String() string {
	return fmt.Sprintf("%s, user=%s", this.Key.DisplayString(), this.User)
}

func (this *Probe) Equals(other *Probe) bool {
	return this.Key.Equals(&other.Key)
}

func (this *Probe) GetDBUri(databaseName string) string {
	hostname := this.Key.Hostname
	var ip = net.ParseIP(hostname)
	if (ip != nil) && (ip.To4() == nil) {
		// Wrap IPv6 literals in square brackets
		hostname = fmt.Sprintf("[%s]", hostname)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&charset=utf8mb4,utf8,latin1&timeout=%dms", this.User, this.Password, hostname, this.Key.Port, databaseName, timeoutMillis)
}
