package throttle

import (
	"net/http"
	"sort"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/mysql"
)

func aggregateMySQLProbes(probes *mysql.Probes, instanceResultsMap mysql.InstanceMetricResultMap, instanceHttpChecksMap mysql.InstanceHttpCheckResultMap, ignoreHostsCount int) (worstMetric base.MetricResult) {
	// probes is known not to change. It can be *replaced*, but not changed.
	// so it's safe to iterate it
	availableProbes := len(*probes)
	for _, probe := range *probes {
		if instanceHttpChecksMap[probe.Key] == http.StatusNotFound {
			availableProbes--
		}
	}
	if availableProbes == 0 {
		return base.NoHostsMetricResult
	}
	probeValues := []float64{}
	for _, probe := range *probes {
		if instanceHttpChecksMap[probe.Key] == http.StatusNotFound {
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
