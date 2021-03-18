package throttle

import (
	"github.com/github/freno/pkg/base"
	"net/http"
)

// CheckResult is the result for an app inquiring on a metric. It also exports as JSON via the API
type CheckResult struct {
	StatusCode int     `json:"StatusCode"`
	Value      float64 `json:"Value"`
	Threshold  float64 `json:"Threshold"`
	Error      error   `json:"-"`
	Message    string  `json:"Message"`
}

func NewCheckResult(statusCode int, value float64, threshold float64, err error) *CheckResult {
	result := &CheckResult{
		StatusCode: statusCode,
		Value:      value,
		Threshold:  threshold,
		Error:      err,
	}
	if err != nil {
		result.Message = err.Error()
	}
	return result
}

func NewErrorCheckResult(statusCode int, err error) *CheckResult {
	return NewCheckResult(statusCode, 0, 0, err)
}

var NoSuchMetricCheckResult = NewErrorCheckResult(http.StatusNotFound, base.NoSuchMetricError)
