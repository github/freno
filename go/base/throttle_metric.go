/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package base

import (
	"errors"
)

type MetricResult interface {
	Get() (float64, error)
}

var noHostsError = errors.New("No hosts found")
var noResultYetError = errors.New("Metric not collected yet")

type noHostsMetricResult struct{}

func (metricResult *noHostsMetricResult) Get() (float64, error) {
	return 0, noHostsError
}

var NoHostsMetricResult = &noHostsMetricResult{}

type noMetricResultYet struct{}

func (metricResult *noMetricResultYet) Get() (float64, error) {
	return 0, noResultYetError
}

var NoMetricResultYet = &noMetricResultYet{}
