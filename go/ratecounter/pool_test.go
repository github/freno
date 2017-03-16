package ratecounter

import (
	"testing"
	"time"

	test "github.com/outbrain/golib/tests"
)

func TestFromPool(t *testing.T) {
	counter := FromPool("myCounter", 1*time.Second)
	anotherCounter := FromPool("myCounter", 1*time.Second)
	aThirdCounter := FromPool("different", 1*time.Second)
	aFourthCounter := FromPool("different", 2*time.Second)

	test.S(t).ExpectEquals(len(pool.counters), 3)
	test.S(t).ExpectEquals(counter, anotherCounter)
	test.S(t).ExpectNotEquals(aThirdCounter, anotherCounter)
	test.S(t).ExpectNotEquals(aThirdCounter, aFourthCounter)
}
