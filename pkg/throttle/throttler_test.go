package throttle

import (
	"net/http"
	"testing"

	"github.com/github/freno/pkg/base"
	"github.com/stretchr/testify/assert"
)

func Test_checkAppMetricResult(t *testing.T) {
	throttler := NewThrottler()

	t.Run("threshold exceeded", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewSimpleMetricResult(10.0), 1.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.Equal(t, 10.0, result.Value)
		assert.EqualError(t, result.Error, base.ThresholdExceededError.Error())
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	})
	t.Run("not throttled", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewSimpleMetricResult(1.0), 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.Equal(t, 1.0, result.Value)
		assert.Nil(t, result.Error)
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})
	t.Run("no hosts", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NoHostsMetricResult, 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{})
		assert.Equal(t, 0.0, result.Value)
		assert.EqualError(t, result.Error, base.NoHostsError.Error())
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})
	t.Run("low priority denied", func(t *testing.T) {
		check := NewThrottlerCheck(throttler)
		throttler.nonLowPriorityAppRequestsThrottled.SetDefault("mysql/test-cluster", true)
		metricResultFunc := func() (metricResult base.MetricResult, threshold float64) {
			return base.NewSimpleMetricResult(1.0), 10.0
		}
		result := check.checkAppMetricResult("test-app", "mysql", "test-cluster", metricResultFunc, &CheckFlags{LowPriority: true})
		assert.EqualError(t, result.Error, base.AppDeniedError.Error())
		assert.Equal(t, http.StatusExpectationFailed, result.StatusCode)

	})
}
