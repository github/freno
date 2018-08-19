/*
   Copyright 2018 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"net/http"
	"time"

	metrics "github.com/rcrowley/go-metrics"
)

var httpClient = http.Client{
	Timeout: 1 * time.Second,
}

type MySQLHttpCheck struct {
	ClusterName string
	Key         InstanceKey
	CheckResult int
}

func NewMySQLHttpCheck(clusterName string, instanceKey *InstanceKey, checkResult int) *MySQLHttpCheck {
	return &MySQLHttpCheck{
		ClusterName: clusterName,
		Key:         *instanceKey,
		CheckResult: checkResult,
	}
}

func (check *MySQLHttpCheck) HashKey() string {
	return MySQLHttpCheckHashKey(check.ClusterName, &check.Key)
}

func MySQLHttpCheckHashKey(clusterName string, key *InstanceKey) string {
	return fmt.Sprintf("%s:%s", clusterName, key.StringCode())
}

func CheckHttp(clusterName string, probe *Probe) (httpCheckResult *MySQLHttpCheck) {

	if probe.HttpCheckPort <= 0 {
		go func() { metrics.GetOrRegisterCounter("httpcheck.skip", nil).Inc(1) }()
		return NewMySQLHttpCheck(clusterName, &probe.Key, http.StatusOK)
	}
	url := fmt.Sprintf("http://%s:%d/%s", probe.Key.Hostname, probe.HttpCheckPort, probe.HttpCheckPath)
	resp, err := httpClient.Get(url)
	if err != nil {
		go func() { metrics.GetOrRegisterCounter("httpcheck.error", nil).Inc(1) }()
		return NewMySQLHttpCheck(clusterName, &probe.Key, http.StatusInternalServerError)
	}
	go func() { metrics.GetOrRegisterCounter(fmt.Sprintf("httpcheck.%d", resp.StatusCode), nil).Inc(1) }()
	return NewMySQLHttpCheck(clusterName, &probe.Key, resp.StatusCode)
}
