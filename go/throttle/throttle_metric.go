/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package throttle

type ThrottleMetric interface {
	MetricValue() float64
}
