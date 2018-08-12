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
	Key         InstanceKey
	CheckResult int
}

func NewMySQLHttpCheck(instanceKey *InstanceKey, checkResult int) *MySQLHttpCheck {
	return &MySQLHttpCheck{
		Key:         *instanceKey,
		CheckResult: checkResult,
	}
}

func CheckHttp(probe *Probe) (httpCheckResult *MySQLHttpCheck) {

	if probe.HttpCheckPort < 0 {
		go func() { metrics.GetOrRegisterCounter("httpcheck.skip", nil).Inc(1) }()
		return NewMySQLHttpCheck(&probe.Key, http.StatusOK)
	}
	url := fmt.Sprintf("%s:%d/%s", probe.Key.Hostname, probe.HttpCheckPort, probe.HttpCheckPath)
	resp, err := httpClient.Get(url)
	if err != nil {
		go func() { metrics.GetOrRegisterCounter("httpcheck.error", nil).Inc(1) }()
		return NewMySQLHttpCheck(&probe.Key, http.StatusInternalServerError)
	}
	go func() { metrics.GetOrRegisterCounter(fmt.Sprintf("httpcheck.%d", resp.StatusCode), nil).Inc(1) }()
	return NewMySQLHttpCheck(&probe.Key, resp.StatusCode)
}
