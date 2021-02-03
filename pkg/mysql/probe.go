/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"time"
)

const (
	maxPoolConnections = 3
	maxIdleConnections = 3
	probeTimeout       = 10 * time.Millisecond
)

// Probe is the minimal configuration required to connect to a MySQL server
type Probe struct {
	Key                 InstanceKey
	Uri                 string
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

// NewProbe allocates memory for a new Probe value and returns its address, or an error in case tlsConfiguration parameters were
// provided, but TLS configuration couldn't be registered. If that's the case, the address of the probe will be nil.
func NewProbe(key *InstanceKey, user, password, databaseName, metricQuery string, cacheMillis int, httpCheckPath string, httpCheckPort int) (*Probe, error) {
	uri, err := MakeUri(key.Hostname, key.Port, user, password, databaseName, probeTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot create probe. Cause:  %w", err)
	}

	p := Probe{
		Key:           *key,
		Uri:           uri,
		MetricQuery:   metricQuery,
		CacheMillis:   cacheMillis,
		HttpCheckPath: httpCheckPath,
		HttpCheckPort: httpCheckPort,
	}

	return &p, nil
}

func (p *Probe) String() string {
	return p.Key.String()
}
