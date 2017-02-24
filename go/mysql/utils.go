/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	gosql "database/sql"
	"fmt"
	"time"

	"github.com/outbrain/golib/sqlutils"
)

type ReplicationLagResult struct {
	Key InstanceKey
	Lag time.Duration
	Err error
}

func NewNoReplicationLagResult() *ReplicationLagResult {
	return &ReplicationLagResult{Lag: 0, Err: nil}
}

func (this *ReplicationLagResult) HasLag() bool {
	return this.Lag > 0
}

// GetReplicationLag returns replication lag for a given connection config; either by explicit query
// or via SHOW SLAVE STATUS
func GetReplicationLag(connectionConfig *ConnectionConfig, replicationLagQuery string) (replicationLag time.Duration, err error) {
	dbUri := connectionConfig.GetDBUri("information_schema")
	var db *gosql.DB
	if db, _, err = sqlutils.GetDB(dbUri); err != nil {
		return replicationLag, err
	}

	if replicationLagQuery != "" {
		var floatLag float64
		err = db.QueryRow(replicationLagQuery).Scan(&floatLag)
		return time.Duration(int64(floatLag*1000)) * time.Millisecond, err
	}

	err = sqlutils.QueryRowsMap(db, `show slave status`, func(m sqlutils.RowMap) error {
		slaveIORunning := m.GetString("Slave_IO_Running")
		slaveSQLRunning := m.GetString("Slave_SQL_Running")
		secondsBehindMaster := m.GetNullInt64("Seconds_Behind_Master")
		if !secondsBehindMaster.Valid {
			return fmt.Errorf("replication not running; Slave_IO_Running=%+v, Slave_SQL_Running=%+v", slaveIORunning, slaveSQLRunning)
		}
		replicationLag = time.Duration(secondsBehindMaster.Int64) * time.Second
		return nil
	})
	return replicationLag, err
}

// GetInstanceKey reads hostname and port on given DB
func GetInstanceKey(db *gosql.DB) (instanceKey *InstanceKey, err error) {
	instanceKey = &InstanceKey{}
	err = db.QueryRow(`select @@global.hostname, @@global.port`).Scan(&instanceKey.Hostname, &instanceKey.Port)
	return instanceKey, err
}
