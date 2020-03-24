/*
   Copyright 2019 GitHub Inc.
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

func TestAggregate(t *testing.T) {
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 0},
		}
		m2 := MetricHealthMap{}
		m1.Aggregate(m2)
		test.S(t).ExpectEquals(len(m1), 1)
		test.S(t).ExpectEquals(m1["a"].SecondsSinceLastHealthy, int64(0))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 0},
		}
		m2 := MetricHealthMap{}
		m2.Aggregate(m1)
		test.S(t).ExpectEquals(len(m2), 1)
		test.S(t).ExpectEquals(m2["a"].SecondsSinceLastHealthy, int64(0))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
		}
		m2 := MetricHealthMap{}
		m1.Aggregate(m2)
		test.S(t).ExpectEquals(len(m1), 1)
		test.S(t).ExpectEquals(m1["a"].SecondsSinceLastHealthy, int64(7))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
		}
		m2 := MetricHealthMap{}
		m2.Aggregate(m1)
		test.S(t).ExpectEquals(len(m2), 1)
		test.S(t).ExpectEquals(m2["a"].SecondsSinceLastHealthy, int64(7))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
		}
		m2 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 11},
		}
		m1.Aggregate(m2)
		test.S(t).ExpectEquals(len(m1), 1)
		test.S(t).ExpectEquals(m1["a"].SecondsSinceLastHealthy, int64(11))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 11},
		}
		m2 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
		}
		m1.Aggregate(m2)
		test.S(t).ExpectEquals(len(m1), 1)
		test.S(t).ExpectEquals(m1["a"].SecondsSinceLastHealthy, int64(11))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
			"b": &MetricHealth{SecondsSinceLastHealthy: 19},
		}
		m2 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 11},
			"b": &MetricHealth{SecondsSinceLastHealthy: 17},
		}
		m1.Aggregate(m2)
		test.S(t).ExpectEquals(len(m1), 2)
		test.S(t).ExpectEquals(m1["a"].SecondsSinceLastHealthy, int64(11))
		test.S(t).ExpectEquals(m1["b"].SecondsSinceLastHealthy, int64(19))
	}
	{
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
			"b": &MetricHealth{SecondsSinceLastHealthy: 19},
		}
		m2 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 11},
			"c": &MetricHealth{SecondsSinceLastHealthy: 17},
		}
		m1.Aggregate(m2)
		test.S(t).ExpectEquals(len(m1), 3)
		test.S(t).ExpectEquals(m1["a"].SecondsSinceLastHealthy, int64(11))
		test.S(t).ExpectEquals(m1["b"].SecondsSinceLastHealthy, int64(19))
		test.S(t).ExpectEquals(m1["c"].SecondsSinceLastHealthy, int64(17))
	}
	{
		m0 := MetricHealthMap{}
		m1 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 7},
			"b": &MetricHealth{SecondsSinceLastHealthy: 19},
		}
		m2 := MetricHealthMap{
			"a": &MetricHealth{SecondsSinceLastHealthy: 11},
			"c": &MetricHealth{SecondsSinceLastHealthy: 17},
		}
		m0.Aggregate(m2)
		m0.Aggregate(m1)
		test.S(t).ExpectEquals(len(m0), 3)
		test.S(t).ExpectEquals(m0["a"].SecondsSinceLastHealthy, int64(11))
		test.S(t).ExpectEquals(m0["b"].SecondsSinceLastHealthy, int64(19))
		test.S(t).ExpectEquals(m0["c"].SecondsSinceLastHealthy, int64(17))
	}
}
