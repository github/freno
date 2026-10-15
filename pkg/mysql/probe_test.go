/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"testing"

	"github.com/github/freno/pkg/config"
	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestNewProbe(t *testing.T) {
	c := NewProbe()
	test.S(t).ExpectEquals(c.Key.Hostname, "")
	test.S(t).ExpectEquals(c.Key.Port, 0)
	test.S(t).ExpectEquals(c.User, "")
	test.S(t).ExpectEquals(c.Password, "")
}

func TestDuplicateCredentials(t *testing.T) {
	c := NewProbe()
	c.Key = InstanceKey{Hostname: "myhost", Port: 3306}
	c.User = "gromit"
	c.Password = "penguin"

	dup := c.DuplicateCredentials(InstanceKey{Hostname: "otherhost", Port: 3310})
	test.S(t).ExpectEquals(dup.Key.Hostname, "otherhost")
	test.S(t).ExpectEquals(dup.Key.Port, 3310)
	test.S(t).ExpectEquals(dup.User, "gromit")
	test.S(t).ExpectEquals(dup.Password, "penguin")
}

func TestDuplicate(t *testing.T) {
	c := NewProbe()
	c.Key = InstanceKey{Hostname: "myhost", Port: 3306}
	c.User = "gromit"
	c.Password = "penguin"

	dup := c.Duplicate()
	test.S(t).ExpectEquals(dup.Key.Hostname, "myhost")
	test.S(t).ExpectEquals(dup.Key.Port, 3306)
	test.S(t).ExpectEquals(dup.User, "gromit")
	test.S(t).ExpectEquals(dup.Password, "penguin")
}

func TestGetDBUri(t *testing.T) {
	c := NewProbe()
	c.Key = InstanceKey{Hostname: "myhost", Port: 3306}
	c.User = "gromit"
	c.Password = "penguin"

	// test default (charset)
	dbUri := c.GetDBUri("test_database")
	test.S(t).ExpectEquals(dbUri, "gromit:penguin@tcp(myhost:3306)/test_database?interpolateParams=true&charset=utf8mb4,utf8,latin1&timeout=1000ms")

	// test setting collation
	config.Settings().Stores.MySQL.Collation = "utf8mb4_unicode_ci"
	dbUri = c.GetDBUri("test_database")
	test.S(t).ExpectEquals(dbUri, "gromit:penguin@tcp(myhost:3306)/test_database?interpolateParams=true&collation=utf8mb4_unicode_ci&timeout=1000ms")
}

func TestCapacityMetricDoesNotFallBackToMySQLQueries(t *testing.T) {
	probe := &Probe{
		Key:                      InstanceKey{Hostname: "127.0.0.1", Port: 3306},
		MetricQuery:              "SELECT 1",
		ProxySQLCapacitySettings: &config.ProxySQLCapacityConfigurationSettings{Metric: "invalid"},
	}
	metric := ReadThrottleMetric(probe, "capacity")
	if metric.Err == nil || metric.Err.Error() != "Metric must be frontend or writer-backend" {
		t.Fatalf("capacity probe fell through to ordinary SQL: %v", metric.Err)
	}
	if metric.ClusterName != "capacity" || metric.Key != probe.Key {
		t.Fatalf("capacity metric lost its cluster/endpoint identity: %+v", metric)
	}
}
