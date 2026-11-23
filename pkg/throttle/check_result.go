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
	StatusCode       int            `json:"StatusCode"`
	Value            float64        `json:"Value"`
	Threshold        float64        `json:"Threshold"`
	MetricName       string         `json:"MetricName,omitempty"`
	Error            error          `json:"-"`
	Message          string         `json:"Message"`
	Dependencies     []*CheckResult `json:"Dependencies,omitempty"`
	rejectionReason  string
	blockingResult   *CheckResult
	measurementError string
}

func (result *CheckResult) MarshalJSON() ([]byte, error) {
	type response CheckResult
	return json.Marshal(struct {
		*response
		MeasurementError   string `json:"MeasurementError,omitempty"`
		BlockingMetricName string `json:"BlockingMetricName,omitempty"`
		RejectionReason    string `json:"RejectionReason,omitempty"`
	}{
		response:           (*response)(result),
		MeasurementError:   result.measurementError,
		BlockingMetricName: result.blockingMetricName(),
		RejectionReason:    result.rejectionResult().reason(),
	})
}

func (result *CheckResult) reason() string {
	if result.StatusCode == http.StatusOK {
		return ""
	}
	if result.rejectionReason != "" {
		return result.rejectionReason
	}
	switch {
	case errors.Is(result.Error, base.RecoveryNotCompleteError):
		return "recovery"
	case errors.Is(result.Error, base.ThresholdExceededError) && result.Value > result.Threshold:
		return "threshold"
	case result.StatusCode == http.StatusInternalServerError:
		return safetyMetricErrorReason(result.Error)
	default:
		return ""
	}
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
		if !errors.Is(err, base.RecoveryNotCompleteError) && !errors.Is(err, base.ThresholdExceededError) {
			result.measurementError = err.Error()
		}
	}
	return result
}

func NewErrorCheckResult(statusCode int, err error) *CheckResult {
	return NewCheckResult(statusCode, 0, 0, err)
}

var NoSuchMetricCheckResult = NewErrorCheckResult(http.StatusNotFound, base.NoSuchMetricError)
