package throttle

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
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
	metricResult, threshold := check.throttler.AppRequestMetricResult(appName, storeName, metricResultFunc, denyApp)
	if flags.OverrideThreshold > 0 {
		threshold = flags.OverrideThreshold
	}
	value, err := metricResult.Get()
	if flags.ReadCheck && flags.OverrideThreshold > 0 && errors.Is(err, base.RecoveryNotCompleteError) && value <= threshold {
		err = nil
	}
	if appName == "" {
		return NewCheckResult(http.StatusExpectationFailed, value, threshold, fmt.Errorf("no app indicated"))
	}

	failOnNoHosts := false
	if storeType == "mysql" {
		if clusterSettings, ok := config.Settings().Stores.MySQL.Clusters[storeName]; ok {
			failOnNoHosts = clusterSettings.FailOnNoHosts
		}
	}
	checkResult = newMetricCheckResult(value, threshold, err, failOnNoHosts)
	if err == nil && value > threshold {
		if !flags.LowPriority && !flags.ReadCheck && appName != frenoAppName {
			// low priority requests will henceforth be denied
			go check.throttler.nonLowPriorityAppRequestsThrottled.SetDefault(metricName, true)
		}
	} else if err == nil && checkResult.StatusCode == http.StatusOK && appName != frenoAppName && check.throttler.getShareDomainSecondsSinceHealth(metricName) >= 1 {
		// throttling based on shared domain metric.
		// we exclude the "freno" app itself, or else this could turn into a snowball: this service ("a") seeing
		// another service ("b") as unhealthy, itself becoming unhealthy, makind b's read into a's state as unheathly,
		// b reporting unhealthy, ad infinitum.
		// The "freno" app is the one to generate those health metrics. It therefore must not participate the
		// shared-domain dependency check.,

		checkResult = NewCheckResult(http.StatusTooManyRequests, value, threshold, base.ThresholdExceededError)
	}
	checkResult.MetricName = metricName
	return checkResult
}

func (check *ThrottlerCheck) checkMySQLCluster(
	appName string,
	clusterName string,
	flags *CheckFlags,
	checkedClusters map[string]bool,
) *CheckResult {
	if checkedClusters[clusterName] {
		return nil
	}
	checkedClusters[clusterName] = true

	metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
		return check.throttler.getMySQLClusterMetrics(clusterName)
	}
	rootResult := check.checkAppMetricResult(appName, "mysql", clusterName, metricResultFunc, flags)
	if rootResult.StatusCode != http.StatusOK {
		return rootResult
	}

	clusterSettings := config.Settings().Stores.MySQL.Clusters[clusterName]
	for _, requiredCluster := range clusterSettings.RequiredClusters {
		requiredFlags := *flags
		requiredFlags.ReadCheck = false
		requiredFlags.OverrideThreshold = 0
		requiredResult := check.checkMySQLCluster(appName, requiredCluster, &requiredFlags, checkedClusters)
		if requiredResult == nil || requiredResult.StatusCode == http.StatusOK {
			continue
		}
		if requiredResult.StatusCode == http.StatusNotFound {
			checkResult := NewErrorCheckResult(
				http.StatusInternalServerError,
				fmt.Errorf("required MySQL cluster metric %q is unavailable", requiredCluster),
			)
			checkResult.MetricName = fmt.Sprintf("mysql/%s", requiredCluster)
			return checkResult
		}
		return requiredResult
	}
	return rootResult
}

// CheckAppStoreMetric
func (check *ThrottlerCheck) Check(appName string, storeType string, storeName string, remoteAddr string, flags *CheckFlags) (checkResult *CheckResult) {
	return check.check(appName, storeType, storeName, remoteAddr, flags, true)
}

func (check *ThrottlerCheck) check(appName string, storeType string, storeName string, remoteAddr string, flags *CheckFlags, observeShadow bool) (checkResult *CheckResult) {
	requestedStoreName := storeName
	var metricResultFunc base.MetricResultFunc
	switch storeType {
	case "mysql":
		{
			mysqlSettings := config.Settings().Stores.MySQL
			_, configured := mysqlSettings.Clusters[storeName]
			if !configured && !flags.OKIfNotExists {
				if fallbackCluster := mysqlSettings.FallbackCluster; fallbackCluster != "" {
					storeName = fallbackCluster
					_, configured = mysqlSettings.Clusters[storeName]
				}
			}
			if configured {
				checkResult = check.checkMySQLCluster(appName, storeName, flags, make(map[string]bool))
				if observeShadow && checkResult.StatusCode == http.StatusOK {
					observeMySQLShadowDependencies("http", storeName, func(name string) *CheckResult {
						metric, threshold := check.throttler.getMySQLClusterMetrics(name)
						return mysqlDependencyCheckResult(name, metric, threshold, false)
					})
				}
			} else {
				metricResultFunc = func() (metricResult base.MetricResult, threshold float64) {
					return base.NoSuchMetric, 0
				}
			}
		}
	}
	if metricResultFunc == nil {
		if checkResult == nil {
			return NoSuchMetricCheckResult
		}
	} else {
		checkResult = check.checkAppMetricResult(appName, storeType, storeName, metricResultFunc, flags)
	}

	go func(statusCode int) {
		metrics.GetOrRegisterCounter("check.any.total", nil).Inc(1)
		metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.total", appName), nil).Inc(1)

		metrics.GetOrRegisterCounter(fmt.Sprintf("check.any.%s.%s.total", storeType, requestedStoreName), nil).Inc(1)
		metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.%s.%s.total", appName, storeType, requestedStoreName), nil).Inc(1)

		if statusCode != http.StatusOK {
			metrics.GetOrRegisterCounter("check.any.error", nil).Inc(1)
			metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.error", appName), nil).Inc(1)

			metrics.GetOrRegisterCounter(fmt.Sprintf("check.any.%s.%s.error", storeType, requestedStoreName), nil).Inc(1)
			metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.%s.%s.error", appName, storeType, requestedStoreName), nil).Inc(1)

			if statusCode == http.StatusInternalServerError {
				metrics.GetOrRegisterCounter("check.any.internal-error", nil).Inc(1)
				metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.internal-error", appName), nil).Inc(1)

				metrics.GetOrRegisterCounter(fmt.Sprintf("check.any.%s.%s.internal-error", storeType, requestedStoreName), nil).Inc(1)
				metrics.GetOrRegisterCounter(fmt.Sprintf("check.%s.%s.%s.internal-error", appName, storeType, requestedStoreName), nil).Inc(1)
			}
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
	checkResult = check.check(frenoAppName, storeType, storeName, "local", StandardCheckFlags, false)
	go check.check(frenoShareDmainAppName, storeType, storeName, "local", StandardCheckFlags, false)

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
