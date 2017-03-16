package ratecounter

import (
	"sync"
	"testing"
)

func TestCounter(t *testing.T) {
	var c Counter

	check := func(expected int64) {
		val := c.Value()
		if val != expected {
			t.Error("Expected ", val, " to equal ", expected)
		}
	}

	check(0)
	c.Incr(1)
	check(1)
	c.Incr(9)
	check(10)

	// Concurrent usage
	wg := &sync.WaitGroup{}
	wg.Add(3)
	for i := 1; i <= 3; i++ {
		go func(val int64) {
			c.Incr(val)
			wg.Done()
		}(int64(i))
	}
	wg.Wait()
	check(16)
}

func BenchmarkCounter(b *testing.B) {
	var c Counter

	for i := 0; i < b.N; i++ {
		c.Incr(1)
	}
}
