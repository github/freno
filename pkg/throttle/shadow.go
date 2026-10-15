package throttle

import (
	"fmt"
	"net/http"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/config"
	metrics "github.com/rcrowley/go-metrics"
)

func mysqlDependencyCheckResult(name string, metric base.MetricResult, threshold float64, background bool) *CheckResult {
	settings := config.Settings().Stores.MySQL.Clusters[name]
	value, err := metric.Get()
	result := newMetricCheckResult(value, threshold, err, background || settings.FailOnNoHosts)
	if result.StatusCode == http.StatusNotFound {
		result = NewErrorCheckResult(http.StatusInternalServerError, fmt.Errorf("required MySQL cluster metric %q is unavailable", name))
	}
	result.MetricName = "mysql/" + name
	return result
}

// Observe only new dependencies. The real required closure already passed and must
// not be resampled or attributed to a shadow failure.
func observeMySQLShadowDependencies(source, root string, evaluate func(string) *CheckResult) {
	clusters := config.Settings().Stores.MySQL.Clusters
	seen := make(map[string]bool)
	var pending []string
	var seed func(string)
	seed = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		pending = append(pending, clusters[name].ShadowRequiredClusters...)
		for _, required := range clusters[name].RequiredClusters {
			seed(required)
		}
	}
	seed(root)
	if len(pending) == 0 {
		return
	}
	prefix := fmt.Sprintf("shadow.%s.mysql.%s", source, root)
	metrics.GetOrRegisterCounter(prefix+".total", nil).Inc(1)
	rejections := metrics.GetOrRegisterCounter(prefix+".would_reject", nil)
	var visit func(string) *CheckResult
	visit = func(name string) *CheckResult {
		if seen[name] {
			return nil
		}
		seen[name] = true
		result := evaluate(name)
		if result.StatusCode != http.StatusOK {
			metrics.GetOrRegisterCounter(prefix+".dependency."+name+".would_reject", nil).Inc(1)
			return result
		}
		for _, names := range [][]string{clusters[name].RequiredClusters, clusters[name].ShadowRequiredClusters} {
			for _, dependency := range names {
				if failure := visit(dependency); failure != nil {
					return failure
				}
			}
		}
		return nil
	}
	for _, name := range pending {
		if failure := visit(name); failure != nil {
			rejections.Inc(1)
			metrics.GetOrRegisterCounter(fmt.Sprintf("%s.status.%d.would_reject", prefix, failure.StatusCode), nil).Inc(1)
			return
		}
	}
}
