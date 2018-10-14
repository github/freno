/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package throttle

import (
	"net/http"
	"testing"

	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestAggregateCheckResults(t *testing.T) {
	throttlerCheck := NewThrottlerCheck(nil)
	{
		checkResults := [](*CheckResult){
			NewCheckResult(http.StatusOK, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
		}
		result := throttlerCheck.aggregateCheckResults(checkResults)
		test.S(t).ExpectEquals(result.StatusCode, http.StatusOK)
	}
	{
		checkResults := [](*CheckResult){
			NewCheckResult(http.StatusOK, 0, 0, nil),
			NewCheckResult(http.StatusNotFound, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
		}
		result := throttlerCheck.aggregateCheckResults(checkResults)
		test.S(t).ExpectEquals(result.StatusCode, http.StatusNotFound)
	}
	{
		checkResults := [](*CheckResult){
			NewCheckResult(http.StatusOK, 0, 0, nil),
			NewCheckResult(http.StatusTooManyRequests, 0, 0, nil),
			NewCheckResult(http.StatusNotFound, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
		}
		result := throttlerCheck.aggregateCheckResults(checkResults)
		test.S(t).ExpectEquals(result.StatusCode, http.StatusTooManyRequests)
	}
	{
		checkResults := [](*CheckResult){
			NewCheckResult(http.StatusNotFound, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
			NewCheckResult(http.StatusTooManyRequests, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
		}
		result := throttlerCheck.aggregateCheckResults(checkResults)
		test.S(t).ExpectEquals(result.StatusCode, http.StatusTooManyRequests)
	}
	{
		checkResults := [](*CheckResult){
			NewCheckResult(http.StatusOK, 0, 0, nil),
			NewCheckResult(http.StatusTooManyRequests, 0, 0, nil),
			NewCheckResult(http.StatusInternalServerError, 0, 0, nil),
			NewCheckResult(http.StatusNotFound, 0, 0, nil),
			NewCheckResult(http.StatusOK, 0, 0, nil),
		}
		result := throttlerCheck.aggregateCheckResults(checkResults)
		test.S(t).ExpectEquals(result.StatusCode, http.StatusTooManyRequests)
	}
}
