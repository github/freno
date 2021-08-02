package throttle

import (
	"net/http"
	"strings"
	"time"

	"fmt"

	"github.com/github/freno/pkg/base"
	metrics "github.com/rcrowley/go-metrics"
)

const frenoAppName = "freno"
const frenoShareDmainAppName = "freno-share-domain"
const selfCheckInterval = 100 * time.Millisecond

type CheckFlags struct {
	ReadCheck         bool
	OverrideThreshold float64
	LowPriority       bool
	OKIfNotExists     bool
}

var StandardCheckFlags = &CheckFlags{}

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
func (check *ThrottlerCheck) checkAppMetricResult(appName string, storeType string, storeName string, metricResultFunc base.MetricResultFunc, flags *CheckFlags) (checkResult *CheckResult) {
	// Handle deprioritized app logic
	denyApp := false
	metricName := fmt.Sprintf("%s/%s", storeType, storeName)
	if flags.LowPriority {
		if _, exists := check.throttler.nonLowPriorityAppRequestsThrottled.Get(metricName); exists {
			// a non-deprioritized app, ie a "normal" app, has recently been throttled.
			// This is now a deprioritized app. Deny access to this request.
			denyApp = true
		}
	}
	//
	metricResult, threshold := check.throttler.AppRequestMetricResult(appName, metricResultFunc, denyApp)
	if flags.OverrideThreshold > 0 {
		threshold = flags.OverrideThreshold
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

		if !flags.LowPriority && !flags.ReadCheck && appName != frenoAppName {
			// low priority requests will henceforth be denied
			go check.throttler.nonLowPriorityAppRequestsThrottled.SetDefault(metricName, true)
		}
	} else if appName != frenoAppName && check.throttler.getShareDomainSecondsSinceHealth(metricName) >= 1 {
		// throttling based on shared domain metric.
		// we exclude the "freno" app itself, or else this could turn into a snowball: this service ("a") seeing
		// another service ("b") as unhealthy, itself becoming unhealthy, makind b's read into a's state as unheathly,
		// b reporting unhealthy, ad infinitum.
		// The "freno" app is the one to generate those health metrics. It therefore must not participate the
		// shared-domain dependency check.,

		statusCode = http.StatusTooManyRequests // 429
		err = base.ThresholdExceededError
	} else {
		// all good!
		statusCode = http.StatusOK // 200
	}
	return NewCheckResult(statusCode, value, threshold, err)
}

// CheckAppStoreMetric
func (check *ThrottlerCheck) Check(appName string, storeType string, storeName string, remoteAddr string, flags *CheckFlags) (checkResult *CheckResult) {
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

	checkResult = check.checkAppMetricResult(appName, storeType, storeName, metricResultFunc, flags)

	go func(statusCode int) {
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
	}(checkResult.StatusCode)

	return checkResult
}

func (check *ThrottlerCheck) splitMetricTokens(metricName string) (storeType string, storeName string, err error) {
	metricTokens := strings.Split(metricName, "/")
	if len(metricTokens) != 2 {
		return storeType, storeName, base.NoSuchMetricError
	}
	storeType = metricTokens[0]
	storeName = metricTokens[1]

	return storeType, storeName, nil
}

// localCheck
func (check *ThrottlerCheck) localCheck(metricName string) (checkResult *CheckResult) {
	storeType, storeName, err := check.splitMetricTokens(metricName)
	if err != nil {
		return NoSuchMetricCheckResult
	}
	checkResult = check.Check(frenoAppName, storeType, storeName, "local", StandardCheckFlags)
	go check.Check(frenoShareDmainAppName, storeType, storeName, "local", StandardCheckFlags)

	if checkResult.StatusCode == http.StatusOK {
		check.throttler.markMetricHealthy(metricName)
	}
	if timeSinceHealthy, found := check.throttler.timeSinceMetricHealthy(metricName); found {
		metrics.GetOrRegisterGauge(fmt.Sprintf("check.%s.%s.seconds_since_healthy", storeType, storeName), nil).Update(int64(timeSinceHealthy.Seconds()))
	}

	return checkResult
}

func (check *ThrottlerCheck) reportAggregated(metricName string, metricResult base.MetricResult) {
	storeType, storeName, err := check.splitMetricTokens(metricName)
	if err != nil {
		return
	}
	if value, err := metricResult.Get(); err == nil {
		metrics.GetOrRegisterGaugeFloat64(fmt.Sprintf("aggregated.%s.%s", storeType, storeName), nil).Update(value)
	}
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
			for metricName, metricResult := range check.AggregatedMetrics() {
				metricName := metricName
				metricResult := metricResult
				go check.localCheck(metricName)
				go check.reportAggregated(metricName, metricResult)
			}
		}
	}()
}
