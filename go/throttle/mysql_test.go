/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package throttle

import (
	"testing"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/mysql"

	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

var (
	key1 = mysql.InstanceKey{Hostname: "10.0.0.1", Port: 3306}
	key2 = mysql.InstanceKey{Hostname: "10.0.0.2", Port: 3306}
	key3 = mysql.InstanceKey{Hostname: "10.0.0.3", Port: 3306}
	key4 = mysql.InstanceKey{Hostname: "10.0.0.4", Port: 3306}
	key5 = mysql.InstanceKey{Hostname: "10.0.0.5", Port: 3306}
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestAggregateMySQLProbesNoErrors(t *testing.T) {
	instanceResultsMap := mysql.InstanceMetricResultMap{
		key1: base.NewSimpleMetricResult(1.2),
		key2: base.NewSimpleMetricResult(1.7),
		key3: base.NewSimpleMetricResult(0.3),
		key4: base.NewSimpleMetricResult(0.6),
		key5: base.NewSimpleMetricResult(1.1),
	}
	var probes mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
	for key := range instanceResultsMap {
		probes[key] = &mysql.Probe{Key: key}
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 1)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.2)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 2)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.1)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 3)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.6)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 4)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.3)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 5)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.3)
	}
}

func TestAggregateMySQLProbesWithErrors(t *testing.T) {
	instanceResultsMap := mysql.InstanceMetricResultMap{
		key1: base.NewSimpleMetricResult(1.2),
		key2: base.NewSimpleMetricResult(1.7),
		key3: base.NewSimpleMetricResult(0.3),
		key4: base.NoSuchMetric,
		key5: base.NewSimpleMetricResult(1.1),
	}
	var probes mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
	for key := range instanceResultsMap {
		probes[key] = &mysql.Probe{Key: key}
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
		test.S(t).ExpectEquals(err, base.NoSuchMetricError)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 1)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 2)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.2)
	}

	instanceResultsMap[key1] = base.NoSuchMetric
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
		test.S(t).ExpectEquals(err, base.NoSuchMetricError)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 1)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
		test.S(t).ExpectEquals(err, base.NoSuchMetricError)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, instanceResultsMap, 2)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
}
