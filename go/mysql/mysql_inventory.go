/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"github.com/github/freno/go/base"
)

type InstanceMetricResultMap map[InstanceKey]base.MetricResult
type ClusterInstanceHttpCheckResultMap map[string]int

type MySQLInventory struct {
	ClustersProbes            map[string](*Probes)
	IgnoreHostsCount          map[string]int
	InstanceKeyMetrics        InstanceMetricResultMap
	ClusterInstanceHttpChecks ClusterInstanceHttpCheckResultMap
}

func NewMySQLInventory() *MySQLInventory {
	inventory := &MySQLInventory{
		ClustersProbes:            make(map[string](*Probes)),
		IgnoreHostsCount:          make(map[string]int),
		InstanceKeyMetrics:        make(map[InstanceKey]base.MetricResult),
		ClusterInstanceHttpChecks: make(map[string]int),
	}
	return inventory
}
