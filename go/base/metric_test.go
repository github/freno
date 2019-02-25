/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package base

import (
	"testing"

	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestGetMetricName(t *testing.T) {
	result := GetMetricName("mysql", "mycluster")
	test.S(t).ExpectEquals(result, "mysql/mycluster")
}

func TestParseMetricName(t *testing.T) {
	{
		storeType, storeName, err := ParseMetricName("mysql/mycluster")
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(storeType, "mysql")
		test.S(t).ExpectEquals(storeName, "mycluster")
	}
	{
		_, _, err := ParseMetricName("mysql")
		test.S(t).ExpectNotNil(err)
	}
	{
		_, _, err := ParseMetricName("freno/mysql/mycluster")
		test.S(t).ExpectNotNil(err)
	}
}
