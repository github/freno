package throttle

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
)

type MySQLMetricExplanation struct {
	MetricName             string
	Mode                   string
	StatusCode             int
	Value                  *float64
	Threshold              float64
	RecoveryThreshold      float64
	RecoveryDurationMillis int
	CacheMillis            int
	FailOnNoHosts          bool
	IgnoreHostsCount       int
	IgnoreHostsThreshold   float64
	IgnoreDialTcpErrors    bool
	Message                string
	RequiredClusters       []string `json:",omitempty"`
	ShadowRequiredClusters []string `json:",omitempty"`
}

type MySQLExplanation struct {
	MetricName string
	Metrics    []MySQLMetricExplanation
}

// ExplainMySQLCluster reads loaded policy and cached metrics without running
// admission checks, sampling probes, or changing recovery and shadow counters.
func (check *ThrottlerCheck) ExplainMySQLCluster(clusterName string) (*MySQLExplanation, error) {
	settings := config.Settings().Stores.MySQL
	if settings.Clusters[clusterName] == nil {
		return nil, base.NoSuchMetricError
	}

	modes := make(map[string]string)
	var visit func(string, string) error
	visit = func(name, mode string) error {
		if previous, found := modes[name]; found && (previous != "shadow" || mode == "shadow") {
			return nil
		}
		cluster := settings.Clusters[name]
		if cluster == nil {
			return fmt.Errorf("missing MySQL dependency configuration for %q", name)
		}
		modes[name] = mode
		requiredMode := "enforced"
		if mode == "shadow" {
			requiredMode = "shadow"
		}
		for _, dependency := range cluster.RequiredClusters {
			if err := visit(dependency, requiredMode); err != nil {
				return err
			}
		}
		for _, dependency := range cluster.ShadowRequiredClusters {
			if err := visit(dependency, "shadow"); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(clusterName, "source"); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(modes)-1)
	for name := range modes {
		if name != clusterName {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append([]string{clusterName}, names...)

	snapshot := check.AggregatedMetrics()
	explanation := &MySQLExplanation{MetricName: "mysql/" + clusterName}
	for _, name := range names {
		cluster := settings.Clusters[name]
		metricName := "mysql/" + name
		metric, found := snapshot[metricName]
		if !found {
			metric = base.NoMetricResultYet
		}
		value, err := metric.Get()
		result := newMetricCheckResult(value, cluster.ThrottleThreshold, err, cluster.FailOnNoHosts)
		if err != nil {
			switch {
			case errors.Is(err, base.RecoveryNotCompleteError):
				result.Message = base.RecoveryNotCompleteError.Error()
			case errors.Is(err, base.NoHostsError):
				result.Message = base.NoHostsError.Error()
			case errors.Is(err, base.NoSuchMetricError):
				result.Message = base.NoSuchMetricError.Error()
			case metric == base.NoMetricResultYet:
				result.Message = "Metric not collected yet"
			default:
				// Probe errors can contain connection details or query text.
				result.Message = "Probe error; consult server logs"
			}
		}
		var observedValue *float64
		if math.IsNaN(value) || math.IsInf(value, 0) {
			result = NewErrorCheckResult(http.StatusInternalServerError, fmt.Errorf("invalid metric value"))
		} else if err == nil || errors.Is(err, base.RecoveryNotCompleteError) {
			observedValue = &value
		}
		recoveryThreshold := cluster.ThrottleThreshold
		if cluster.RecoveryThreshold != nil {
			recoveryThreshold = *cluster.RecoveryThreshold
		}
		explanation.Metrics = append(explanation.Metrics, MySQLMetricExplanation{
			MetricName:             metricName,
			Mode:                   modes[name],
			StatusCode:             result.StatusCode,
			Value:                  observedValue,
			Threshold:              cluster.ThrottleThreshold,
			RecoveryThreshold:      recoveryThreshold,
			RecoveryDurationMillis: cluster.RecoveryDurationMillis,
			CacheMillis:            cluster.CacheMillis,
			FailOnNoHosts:          cluster.FailOnNoHosts,
			IgnoreHostsCount:       cluster.IgnoreHostsCount,
			IgnoreHostsThreshold:   cluster.IgnoreHostsThreshold,
			IgnoreDialTcpErrors:    settings.IgnoreDialTcpErrors,
			Message:                result.Message,
			RequiredClusters:       cluster.RequiredClusters,
			ShadowRequiredClusters: cluster.ShadowRequiredClusters,
		})
	}
	return explanation, nil
}
