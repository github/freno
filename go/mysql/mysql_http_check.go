/*
   Copyright 2018 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"fmt"
	"net/http"
)

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
		return NewMySQLHttpCheck(&probe.Key, http.StatusOK)
	}
	url := fmt.Sprintf("%s:%d/%s", probe.Key.Hostname, probe.HttpCheckPort, probe.HttpCheckPath)
	resp, err := http.Get(url)
	if err != nil {
		return NewMySQLHttpCheck(&probe.Key, http.StatusInternalServerError)
	}
	return NewMySQLHttpCheck(&probe.Key, resp.StatusCode)
}
