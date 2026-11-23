package throttle

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/github/freno/pkg/base"
)

func newMetricCheckResult(value, threshold float64, err error, failOnNoHosts bool) *CheckResult {
	status := http.StatusOK
	switch {
	case err == base.AppDeniedError:
		status = http.StatusExpectationFailed
	case err == base.NoSuchMetricError:
		status = http.StatusNotFound
	case errors.Is(err, base.NoHostsError):
		if failOnNoHosts {
			status = http.StatusInternalServerError
		}
	case errors.Is(err, base.RecoveryNotCompleteError):
		status = http.StatusTooManyRequests
	case err != nil:
		status = http.StatusInternalServerError
	case value > threshold:
		status = http.StatusTooManyRequests
		err = base.ThresholdExceededError
	}
	return NewCheckResult(status, value, threshold, err)
}

// CheckResult is the result for an app inquiring on a metric. It also exports as JSON via the API
type CheckResult struct {
	StatusCode      int            `json:"StatusCode"`
	Value           float64        `json:"Value"`
	Threshold       float64        `json:"Threshold"`
	MetricName      string         `json:"MetricName,omitempty"`
	Error           error          `json:"-"`
	Message         string         `json:"Message"`
	Dependencies    []*CheckResult `json:"Dependencies,omitempty"`
	rejectionReason string
	blockingResult  *CheckResult
}

type checkResultResponse struct {
	StatusCode   int                    `json:"StatusCode"`
	Value        float64                `json:"Value"`
	Threshold    float64                `json:"Threshold"`
	MetricName   string                 `json:"MetricName,omitempty"`
	Message      string                 `json:"Message"`
	Dependencies []*checkResultResponse `json:"Dependencies,omitempty"`
}

func (result *CheckResult) response() *checkResultResponse {
	if result == nil {
		return nil
	}
	response := &checkResultResponse{
		StatusCode: result.StatusCode,
		Value:      result.Value,
		Threshold:  result.Threshold,
		MetricName: result.MetricName,
		Message:    result.Message,
	}
	for _, dependency := range result.Dependencies {
		response.Dependencies = append(response.Dependencies, dependency.response())
	}
	return response
}

func (result *CheckResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		*checkResultResponse
		BlockingMetricName string `json:"BlockingMetricName,omitempty"`
	}{
		checkResultResponse: result.response(),
		BlockingMetricName:  result.blockingMetricName(),
	})
}

func (result *CheckResult) rejectionResult() *CheckResult {
	if result.blockingResult != nil {
		return result.blockingResult
	}
	return result
}

func (result *CheckResult) blockingMetricName() string {
	if result.StatusCode == http.StatusOK {
		return ""
	}
	return result.rejectionResult().MetricName
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
