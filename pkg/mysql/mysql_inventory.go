/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"github.com/github/freno/pkg/base"
)

type ClusterInstanceKey struct {
	ClusterName string
	Key         InstanceKey
}

func GetClusterInstanceKey(clusterName string, key *InstanceKey) ClusterInstanceKey {
	return ClusterInstanceKey{ClusterName: clusterName, Key: *key}
}

func (c ClusterInstanceKey) HashCode() string {
	return fmt.Sprintf("%s:%s", c.ClusterName, c.Key.StringCode())
}

type InstanceMetricResultMap map[ClusterInstanceKey]base.MetricResult
type ClusterInstanceHttpCheckResultMap map[string]int

type MySQLInventory struct {
	ClustersProbes            map[string](*Probes)
	IgnoreHostsCount          map[string]int
	IgnoreHostsThreshold      map[string]float64
	InstanceKeyMetrics        InstanceMetricResultMap
	ClusterInstanceHttpChecks ClusterInstanceHttpCheckResultMap
}

func NewMySQLInventory() *MySQLInventory {
	inventory := &MySQLInventory{
		ClustersProbes:            make(map[string](*Probes)),
		IgnoreHostsCount:          make(map[string]int),
		IgnoreHostsThreshold:      make(map[string]float64),
		InstanceKeyMetrics:        make(map[ClusterInstanceKey]base.MetricResult),
		ClusterInstanceHttpChecks: make(map[string]int),
	}
	return inventory
}
