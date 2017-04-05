package throttle

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
