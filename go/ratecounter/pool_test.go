package ratecounter

import (
	"testing"
	"time"

	test "github.com/outbrain/golib/tests"
)

func TestFromPoolKeepsASingleInstanceOfTheSameCounter(t *testing.T) {
	counter := FromPool("myCounter", 1*time.Second)
	anotherCounter := FromPool("myCounter", 1*time.Second)
	aThirdCounter := FromPool("different", 1*time.Second)

	test.S(t).ExpectEquals(len(pool.counters), 2)
	test.S(t).ExpectEquals(counter, anotherCounter)
	test.S(t).ExpectNotEquals(aThirdCounter, anotherCounter)
}

func TestFromPoolPanicsIfSameCountersWithDifferentDuration(t *testing.T) {
	defer func() {
		recover()
	}()
	FromPool("myCounter", 1*time.Second)
	FromPool("myCounter", 2*time.Second)
	test.S(t).Errorf("Should have panic'ed")
}

func TestIncr(t *testing.T) {
	counter := FromPool("myCounter", 1*time.Second)
	counter.Incr(3)
	test.S(t).ExpectEquals(counter.rateCounter.Rate(), int64(3))
	test.S(t).ExpectEquals(counter.expvarInt.String(), "3")
}
