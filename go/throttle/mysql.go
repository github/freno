package throttle

import (
	"net/http"
	"sort"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/mysql"
)

func aggregateMySQLProbes(probes *mysql.Probes, clusterName string, instanceResultsMap mysql.InstanceMetricResultMap, clusterInstanceHttpChecksMap mysql.ClusterInstanceHttpCheckResultMap, ignoreHostsCount int) (worstMetric base.MetricResult) {
	// probes is known not to change. It can be *replaced*, but not changed.
	// so it's safe to iterate it
	probeValues := []float64{}
	for _, probe := range *probes {
		if clusterInstanceHttpChecksMap[mysql.MySQLHttpCheckHashKey(clusterName, &probe.Key)] == http.StatusNotFound {
			continue
		}
		instanceMetricResult, ok := instanceResultsMap[probe.Key]
		if !ok {
			return base.NoMetricResultYet
		}

		value, err := instanceMetricResult.Get()
		if err != nil {
			if ignoreHostsCount > 0 {
				// ok to skip this error
				ignoreHostsCount = ignoreHostsCount - 1
				continue
			}
			return instanceMetricResult
		}

		// No error
		probeValues = append(probeValues, value)
	}
	if len(probeValues) == 0 {
		return base.NoHostsMetricResult
	}

	// If we got here, that means no errors (or good to skip errors)
	sort.Float64s(probeValues)
	for ignoreHostsCount > 0 {
		if len(probeValues) > 1 {
			probeValues = probeValues[0 : len(probeValues)-1]
		}
		ignoreHostsCount = ignoreHostsCount - 1
	}
	worstValue := probeValues[len(probeValues)-1]
	worstMetric = base.NewSimpleMetricResult(worstValue)
	return worstMetric
}
