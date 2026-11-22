package throttle

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	"github.com/outbrain/golib/log"
	"github.com/patrickmn/go-cache"
	metrics "github.com/rcrowley/go-metrics"
)

func safetyMetricErrorReason(err error) string {
	switch {
	case errors.Is(err, base.NoHostsError):
		return "no_hosts"
	case errors.Is(err, base.NoSuchMetricError):
		return "missing_metric"
	case errors.Is(err, base.RecoveryNotCompleteError):
		return "recovery"
	default:
		return "metric_error"
	}
}

func (check *ThrottlerCheck) recordMySQLRejection(appName, clusterName string, result *CheckResult) {
	if appName == frenoAppName || appName == frenoShareDmainAppName ||
		(result.StatusCode != http.StatusTooManyRequests && result.StatusCode != http.StatusInternalServerError) {
		return
	}
	// Use configured names, including the resolved fallback, rather than unbounded caller input.
	clusters := config.Settings().Stores.MySQL.Clusters
	dependency := strings.TrimPrefix(result.MetricName, "mysql/")
	if clusters[clusterName] == nil || clusters[dependency] == nil {
		return
	}
	reason := result.rejectionReason
	if reason == "" {
		switch {
		case result.StatusCode == http.StatusInternalServerError:
			reason = safetyMetricErrorReason(result.Error)
		case errors.Is(result.Error, base.RecoveryNotCompleteError):
			reason = "recovery"
		case errors.Is(result.Error, base.ThresholdExceededError) && result.Value > result.Threshold:
			reason = "threshold"
		default:
			reason = "other"
		}
	}
	name := fmt.Sprintf("check.rejection.mysql.%s.dependency.%s.status.%d.%s.total",
		clusterName, dependency, result.StatusCode, reason)
	metrics.GetOrRegisterCounter(name, nil).Inc(1)
	if check.throttler.safetyRejectionLogs.Add(name, true, cache.DefaultExpiration) == nil {
		log.Infof("event=freno_admission_rejected cluster=%q dependency=%q status=%d reason=%q value=%g threshold=%g",
			clusterName, dependency, result.StatusCode, reason, result.Value, result.Threshold)
	}
}

func reportRecoveryTransition(clusterName, event, reason string, value float64, settings *config.MySQLClusterConfigurationSettings) {
	// check.* counters are already collected as rates by the production Go Expvar integration.
	name := fmt.Sprintf("check.recovery.mysql.%s.%s.total", clusterName, event)
	metrics.GetOrRegisterCounter(name, nil).Inc(1)
	recoveryThreshold := settings.ThrottleThreshold
	if settings.RecoveryThreshold != nil {
		recoveryThreshold = *settings.RecoveryThreshold
	}
	log.Infof("event=freno_recovery_%s cluster=%q reason=%q value=%g threshold=%g recovery_threshold=%g recovery_duration_ms=%d",
		event, clusterName, reason, value, settings.ThrottleThreshold, recoveryThreshold, settings.RecoveryDurationMillis)
}
