package throttle

import (
	"net/http"

	"fmt"
	"github.com/github/freno/go/base"
	metrics "github.com/rcrowley/go-metrics"
)

const frenoAppName = "freno"

// ThrottlerCheck provides methdos for an app checking on metrics
type ThrottlerCheck struct {
	throttler *Throttler
}

func NewThrottlerCheck(throttler *Throttler) *ThrottlerCheck {
	return &ThrottlerCheck{
		throttler: throttler,
	}
}

// checkAppMetricResult allows an app to check on a metric
func (check *ThrottlerCheck) checkAppMetricResult(appName string, metricResultFunc base.MetricResultFunc) (checkResult *CheckResult) {
	metricResult, threshold := check.throttler.AppRequestMetricResult(appName, metricResultFunc)
	value, err := metricResult.Get()
	if appName == "" {
		return NewCheckResult(http.StatusExpectationFailed, value, threshold, fmt.Errorf("no app indicated"))
	}

	statusCode := http.StatusInternalServerError // 500

	defer func(appName string, statusCode *int) {
		go func() {
			metrics.GetOrRegisterCounter("check.any.total", nil).Inc(1)
			metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.total", appName), nil).Inc(1)
			if *statusCode != http.StatusOK {
				metrics.GetOrRegisterCounter("check.any.error", nil).Inc(1)
				metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.error", appName), nil).Inc(1)
			}
		}()
	}(appName, &statusCode)

	if err == base.AppDeniedError {
		// app specifically not allowed to get metrics
		statusCode = http.StatusExpectationFailed // 417
	} else if err == base.NoSuchMetricError {
		// not collected yet, or metric does not exist
		statusCode = http.StatusNotFound // 404
	} else if err != nil {
		// any error
		statusCode = http.StatusInternalServerError // 500
	} else if value > threshold {
		// casual throttling
		statusCode = http.StatusTooManyRequests // 429
		err = base.ThresholdExceededError
	} else {
		// all good!
		statusCode = http.StatusOK // 200
	}
	return NewCheckResult(statusCode, value, threshold, err)
}

// CheckMySQLCluster allows an app to check on a MySQL cluster
func (check *ThrottlerCheck) CheckMySQLCluster(appName string, clusterName string) (checkResult *CheckResult) {
	var metricResultFunc base.MetricResultFunc = func() (metricResult base.MetricResult, threshold float64) {
		return check.throttler.GetMySQLClusterMetrics(clusterName)
	}
	return check.checkAppMetricResult(appName, metricResultFunc)
}

// AggregatedMetrics is a convenience acces method into throttler's `aggregatedMetricsSnapshot`
func (check *ThrottlerCheck) AggregatedMetrics() map[string]base.MetricResult {
	return check.throttler.aggregatedMetricsSnapshot()
}
