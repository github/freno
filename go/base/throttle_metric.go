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

type MetricResultFunc func() (metricResult MetricResult, threshold float64)

var ThresholdExceededError = errors.New("Threshold exceeded")
var noHostsError = errors.New("No hosts found")
var noResultYetError = errors.New("Metric not collected yet")
var noSuchMetricError = errors.New("No such metric")

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

type noSuchMetric struct{}

func (metricResult *noSuchMetric) Get() (float64, error) {
	return 0, noSuchMetricError
}

var NoSuchMetric = &noSuchMetric{}
