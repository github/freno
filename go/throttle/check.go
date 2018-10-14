package throttle

import (
	"net/http"
	"time"

	"fmt"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/config"

	metrics "github.com/rcrowley/go-metrics"
)

const frenoAppName = "freno"
const selfCheckInterval = 1 * time.Second

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
func (check *ThrottlerCheck) checkAppMetricResult(appName string, metricResultFunc base.MetricResultFunc, overrideThreshold float64) (checkResult *CheckResult) {
	metricResult, threshold := check.throttler.AppRequestMetricResult(appName, metricResultFunc)
	if overrideThreshold > 0 {
		threshold = overrideThreshold
	}
	value, err := metricResult.Get()
	if appName == "" {
		return NewCheckResult(http.StatusExpectationFailed, value, threshold, fmt.Errorf("no app indicated"))
	}

	statusCode := http.StatusInternalServerError // 500

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

// aggregateCheckResults reviews multiple check results and returns one result, aiming to return the worst result
func (check *ThrottlerCheck) aggregateCheckResults(checkResults []*CheckResult) (checkResult *CheckResult) {
	for _, checkResult = range checkResults {
		status := checkResult.StatusCode
		// First, look for really bad results
		if status != http.StatusOK && status != http.StatusNotFound {
			return checkResult
		}
	}
	for _, checkResult = range checkResults {
		status := checkResult.StatusCode
		// Now also allow 404, which upper layers may choose to ignore
		if status != http.StatusOK {
			return checkResult
		}
	}
	for _, checkResult = range checkResults {
		// Now return any good result
		return checkResult
	}
	return NoSuchMetricCheckResult
}

func (check *ThrottlerCheck) reportMetrics(appName, storeType, storeName, remoteAddr string, statusCode int) {
	metrics.GetOrRegisterCounter("check.any.total", nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.total", appName), nil).Inc(1)

	metrics.GetOrRegisterCounter(fmt.Sprintf("check.any.%s.%s.total", storeType, storeName), nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.%s.%s.total", appName, storeType, storeName), nil).Inc(1)

	if statusCode != http.StatusOK {
		metrics.GetOrRegisterCounter("check.any.error", nil).Inc(1)
		metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.error", appName), nil).Inc(1)

		metrics.GetOrRegisterCounter(fmt.Sprintf("check.any.%s.%s.error", storeType, storeName), nil).Inc(1)
		metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.%s.%s.error", appName, storeType, storeName), nil).Inc(1)
	}

	check.throttler.markRecentApp(appName, remoteAddr)
}

// CheckAppStoreMetric
func (check *ThrottlerCheck) Check(appName string, storeType string, storeName string, remoteAddr string, overrideThreshold float64) (checkResult *CheckResult) {
	metricName := base.GetMetricName(storeType, storeName)
	if value, ok := check.throttler.metaChecks.Get(metricName); ok {
		// This is a meta-check
		checksListing, _ := value.(config.MetaChecksListing)
		var checkResults [](*CheckResult)
		for _, subMetricName := range checksListing {
			if subMetricName == metricName {
				// infinite loop
				return NoSuchMetricCheckResult
			}
			subStoreType, subStoreName, err := base.ParseMetricName(subMetricName)
			if err != nil {
				return NoSuchMetricCheckResult
			}
			// recurse
			// overrideThreshold is forwarded to recursion, and should be carefully considered.
			checkResults = append(checkResults, check.Check(appName, subStoreType, subStoreName, remoteAddr, overrideThreshold))
		}
		checkResult = check.aggregateCheckResults(checkResults)
		go check.reportMetrics(appName, storeType, storeName, remoteAddr, checkResult.StatusCode)
		return checkResult
	}

	var metricResultFunc base.MetricResultFunc
	switch storeType {
	case "mysql":
		{
			metricResultFunc = func() (metricResult base.MetricResult, threshold float64) {
				return check.throttler.getMySQLClusterMetrics(storeName)
			}
		}
	}
	if metricResultFunc == nil {
		return NoSuchMetricCheckResult
	}

	checkResult = check.checkAppMetricResult(appName, metricResultFunc, overrideThreshold)
	go check.reportMetrics(appName, storeType, storeName, remoteAddr, checkResult.StatusCode)
	return checkResult
}

// localCheck
func (check *ThrottlerCheck) localCheck(appName string, metricName string) (checkResult *CheckResult) {
	storeType, storeName, err := base.ParseMetricName(metricName)
	if err != nil {
		return NoSuchMetricCheckResult
	}
	checkResult = check.Check(appName, storeType, storeName, "local", 0)

	if checkResult.StatusCode == http.StatusOK {
		check.throttler.markMetricHealthy(metricName)
	}
	if timeSinceHealthy, found := check.throttler.timeSinceMetricHealthy(metricName); found {
		metrics.GetOrRegisterGauge(fmt.Sprintf("check.%s.%s.seconds_since_healthy", storeType, storeName), nil).Update(int64(timeSinceHealthy.Seconds()))
	}

	return checkResult
}

// AggregatedMetrics is a convenience acces method into throttler's `aggregatedMetricsSnapshot`
func (check *ThrottlerCheck) AggregatedMetrics() map[string]base.MetricResult {
	return check.throttler.aggregatedMetricsSnapshot()
}

// MetricsHealth is a convenience acces method into throttler's `metricsHealthSnapshot`
func (check *ThrottlerCheck) MetricsHealth() map[string](*base.MetricHealth) {
	return check.throttler.metricsHealthSnapshot()
}

func (check *ThrottlerCheck) SelfChecks() {
	selfCheckTick := time.Tick(selfCheckInterval)
	go func() {
		for range selfCheckTick {
			for metricName := range check.AggregatedMetrics() {
				metricName := metricName
				go check.localCheck(frenoAppName, metricName)
			}
		}
	}()
}
