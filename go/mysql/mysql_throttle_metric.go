/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	gosql "database/sql"
	"fmt"
	"strings"

	"github.com/outbrain/golib/sqlutils"
)

type MySQLThrottleMetric struct {
	Key   InstanceKey
	Value float64
}

func NewMySQLThrottleMetric() *MySQLThrottleMetric {
	return &MySQLThrottleMetric{Value: 0}
}

func (mySQLThrottleMetric *MySQLThrottleMetric) MetricValue() float64 {
	return mySQLThrottleMetric.Value
}

// GetReplicationLag returns replication lag for a given connection config; either by explicit query
// or via SHOW SLAVE STATUS
func ReadThrottleMetric(connectionConfig *ConnectionConfig, metricQuery string) (mySQLThrottleMetric *MySQLThrottleMetric, err error) {
	mySQLThrottleMetric = NewMySQLThrottleMetric()
	mySQLThrottleMetric.Key = connectionConfig.Key

	dbUri := connectionConfig.GetDBUri("information_schema")
	var db *gosql.DB
	if db, _, err = sqlutils.GetDB(dbUri); err != nil {
		return mySQLThrottleMetric, err
	}

	if strings.HasPrefix(strings.ToLower(metricQuery), "select") {
		err = db.QueryRow(metricQuery).Scan(&mySQLThrottleMetric.Value)
		return mySQLThrottleMetric, err
	}

	if strings.HasPrefix(strings.ToLower(metricQuery), "show global") {
		var variableName string
		err = db.QueryRow(metricQuery).Scan(&variableName, &mySQLThrottleMetric.Value)
		return mySQLThrottleMetric, err
	}

	if metricQuery != "" {
		return mySQLThrottleMetric, fmt.Errorf("Unsupported metrics query type: %s", metricQuery)
	}

	// No metric query? By default we look at replication lag as output of SHOW SLAVE STATUS

	err = sqlutils.QueryRowsMap(db, `show slave status`, func(m sqlutils.RowMap) error {
		slaveIORunning := m.GetString("Slave_IO_Running")
		slaveSQLRunning := m.GetString("Slave_SQL_Running")
		secondsBehindMaster := m.GetNullInt64("Seconds_Behind_Master")
		if !secondsBehindMaster.Valid {
			return fmt.Errorf("replication not running; Slave_IO_Running=%+v, Slave_SQL_Running=%+v", slaveIORunning, slaveSQLRunning)
		}
		mySQLThrottleMetric.Value = float64(secondsBehindMaster.Int64)
		return nil
	})
	return mySQLThrottleMetric, err
}
