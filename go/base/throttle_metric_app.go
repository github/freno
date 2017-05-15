/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package base

import (
	"errors"
)

var AppDeniedError = errors.New("App denied")

type appDeniedMetric struct{}

func (metricResult *appDeniedMetric) Get() (float64, error) {
	return 0, AppDeniedError
}

var AppDeniedMetric = &appDeniedMetric{}
