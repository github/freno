/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package throttle

import (
	"net/http"
	"testing"

	"github.com/github/freno/pkg/base"
	"github.com/github/freno/pkg/mysql"

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
	clusterName := "c0"
	key1cluster := mysql.GetClusterInstanceKey(clusterName, &key1)
	key2cluster := mysql.GetClusterInstanceKey(clusterName, &key2)
	key3cluster := mysql.GetClusterInstanceKey(clusterName, &key3)
	key4cluster := mysql.GetClusterInstanceKey(clusterName, &key4)
	key5cluster := mysql.GetClusterInstanceKey(clusterName, &key5)
	instanceResultsMap := mysql.InstanceMetricResultMap{
		key1cluster: base.NewSimpleMetricResult(1.2),
		key2cluster: base.NewSimpleMetricResult(1.7),
		key3cluster: base.NewSimpleMetricResult(0.3),
		key4cluster: base.NewSimpleMetricResult(0.6),
		key5cluster: base.NewSimpleMetricResult(1.1),
	}
	clusterInstanceHttpCheckResultMap := mysql.ClusterInstanceHttpCheckResultMap{
		mysql.MySQLHttpCheckHashKey(clusterName, &key1): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key2): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key3): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key4): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key5): http.StatusOK,
	}
	var probes mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
	for clusterKey := range instanceResultsMap {
		probes[clusterKey.Key] = &mysql.Probe{Key: clusterKey.Key}
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 1, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.2)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 2, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.1)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 3, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.6)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 4, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.3)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 5, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.3)
	}
}

func TestAggregateMySQLProbesNoErrorsIgnoreHostsThreshold(t *testing.T) {
	clusterName := "c0"
	key1cluster := mysql.GetClusterInstanceKey(clusterName, &key1)
	key2cluster := mysql.GetClusterInstanceKey(clusterName, &key2)
	key3cluster := mysql.GetClusterInstanceKey(clusterName, &key3)
	key4cluster := mysql.GetClusterInstanceKey(clusterName, &key4)
	key5cluster := mysql.GetClusterInstanceKey(clusterName, &key5)
	instanceResultsMap := mysql.InstanceMetricResultMap{
		key1cluster: base.NewSimpleMetricResult(1.2),
		key2cluster: base.NewSimpleMetricResult(1.7),
		key3cluster: base.NewSimpleMetricResult(0.3),
		key4cluster: base.NewSimpleMetricResult(0.6),
		key5cluster: base.NewSimpleMetricResult(1.1),
	}
	clusterInstanceHttpCheckResultMap := mysql.ClusterInstanceHttpCheckResultMap{
		mysql.MySQLHttpCheckHashKey(clusterName, &key1): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key2): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key3): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key4): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key5): http.StatusOK,
	}
	var probes mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
	for clusterKey := range instanceResultsMap {
		probes[clusterKey.Key] = &mysql.Probe{Key: clusterKey.Key}
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 1.0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 1, false, 1.0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.2)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 2, false, 1.0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.1)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 3, false, 1.0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.6)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 4, false, 1.0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.6)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 5, false, 1.0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 0.6)
	}
}

func TestAggregateMySQLProbesWithErrors(t *testing.T) {
	clusterName := "c0"
	key1cluster := mysql.GetClusterInstanceKey(clusterName, &key1)
	key2cluster := mysql.GetClusterInstanceKey(clusterName, &key2)
	key3cluster := mysql.GetClusterInstanceKey(clusterName, &key3)
	key4cluster := mysql.GetClusterInstanceKey(clusterName, &key4)
	key5cluster := mysql.GetClusterInstanceKey(clusterName, &key5)
	instanceResultsMap := mysql.InstanceMetricResultMap{
		key1cluster: base.NewSimpleMetricResult(1.2),
		key2cluster: base.NewSimpleMetricResult(1.7),
		key3cluster: base.NewSimpleMetricResult(0.3),
		key4cluster: base.NoSuchMetric,
		key5cluster: base.NewSimpleMetricResult(1.1),
	}
	clusterInstanceHttpCheckResultMap := mysql.ClusterInstanceHttpCheckResultMap{
		mysql.MySQLHttpCheckHashKey(clusterName, &key1): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key2): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key3): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key4): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key5): http.StatusOK,
	}
	var probes mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
	for clusterKey := range instanceResultsMap {
		probes[clusterKey.Key] = &mysql.Probe{Key: clusterKey.Key}
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
		test.S(t).ExpectEquals(err, base.NoSuchMetricError)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 1, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 2, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.2)
	}

	instanceResultsMap[key1cluster] = base.NoSuchMetric
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
		test.S(t).ExpectEquals(err, base.NoSuchMetricError)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 1, false, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
		test.S(t).ExpectEquals(err, base.NoSuchMetricError)
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 2, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.7)
	}
}

func TestAggregateMySQLProbesWithHttpChecks(t *testing.T) {
	clusterName := "c0"
	key1cluster := mysql.GetClusterInstanceKey(clusterName, &key1)
	key2cluster := mysql.GetClusterInstanceKey(clusterName, &key2)
	key3cluster := mysql.GetClusterInstanceKey(clusterName, &key3)
	key4cluster := mysql.GetClusterInstanceKey(clusterName, &key4)
	key5cluster := mysql.GetClusterInstanceKey(clusterName, &key5)
	instanceResultsMap := mysql.InstanceMetricResultMap{
		key1cluster: base.NewSimpleMetricResult(1.2),
		key2cluster: base.NewSimpleMetricResult(1.7),
		key3cluster: base.NewSimpleMetricResult(0.3),
		key4cluster: base.NoSuchMetric,
		key5cluster: base.NewSimpleMetricResult(1.1),
	}
	clusterInstanceHttpCheckResultMap := mysql.ClusterInstanceHttpCheckResultMap{
		mysql.MySQLHttpCheckHashKey(clusterName, &key1): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key2): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key3): http.StatusOK,
		mysql.MySQLHttpCheckHashKey(clusterName, &key4): http.StatusNotFound,
		mysql.MySQLHttpCheckHashKey(clusterName, &key5): http.StatusOK,
	}
	var probes mysql.Probes = map[mysql.InstanceKey](*mysql.Probe){}
	for clusterKey := range instanceResultsMap {
		probes[clusterKey.Key] = &mysql.Probe{Key: clusterKey.Key}
	}
	{
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
	}
	{
		clusterInstanceHttpCheckResultMap[mysql.MySQLHttpCheckHashKey(clusterName, &key2)] = http.StatusNotFound
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 0)
		value, err := worstMetric.Get()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(value, 1.2)
	}
	{
		for hashKey := range clusterInstanceHttpCheckResultMap {
			clusterInstanceHttpCheckResultMap[hashKey] = http.StatusNotFound
		}
		worstMetric := aggregateMySQLProbes(&probes, clusterName, instanceResultsMap, clusterInstanceHttpCheckResultMap, 0, false, 0)
		_, err := worstMetric.Get()
		test.S(t).ExpectNotNil(err)
	}
}
