/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"github.com/github/freno/go/base"
)

type MySQLInventory struct {
	ClustersProbes     map[string](*Probes)
	InstanceKeyMetrics map[InstanceKey]base.MetricResult
}

func NewMySQLInventory() *MySQLInventory {
	inventory := &MySQLInventory{
		ClustersProbes:     make(map[string](*Probes)),
		InstanceKeyMetrics: make(map[InstanceKey]base.MetricResult),
	}
	return inventory
}
