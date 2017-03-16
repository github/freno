package ratecounter

import (
	"fmt"
	"io/ioutil"
	"testing"
	"time"
)

func TestRateCounter(t *testing.T) {
	interval := 500 * time.Millisecond
	r := NewRateCounter(interval)

	check := func(expected int64) {
		val := r.Rate()
		if val != expected {
			t.Error("Expected ", val, " to equal ", expected)
		}
	}

	check(0)
	r.Incr(1)
	check(1)
	r.Incr(2)
	check(3)
	time.Sleep(2 * interval)
	check(0)
}

func TestRateCounter_Incr_ReturnsImmediately(t *testing.T) {
	interval := 1 * time.Second
	r := NewRateCounter(interval)

	start := time.Now()
	r.Incr(-1)
	duration := time.Since(start)

	if duration >= 1*time.Second {
		t.Error("incr took", duration, "to return")
	}
}

func BenchmarkRateCounter(b *testing.B) {
	interval := 0 * time.Millisecond
	r := NewRateCounter(interval)

	for i := 0; i < b.N; i++ {
		r.Incr(1)
		r.Rate()
	}
}

func BenchmarkRateCounter_Parallel(b *testing.B) {
	interval := 0 * time.Millisecond
	r := NewRateCounter(interval)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Incr(1)
			r.Rate()
		}
	})
}

func BenchmarkRateCounter_With5MillionExisting(b *testing.B) {
	interval := 1 * time.Hour
	r := NewRateCounter(interval)

	for i := 0; i < 5000000; i++ {
		r.Incr(1)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Incr(1)
		r.Rate()
	}
}

func Benchmark_TimeNowAndAdd(b *testing.B) {
	a := []time.Time{}
	for i := 0; i < b.N; i++ {
		a = append(a, time.Now().Add(1*time.Second))
	}
	fmt.Fprintln(ioutil.Discard, a)
}
