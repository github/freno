package ratecounter

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	rc "github.com/paulbellamy/ratecounter"
)

// ExpvarRateCounter groups together a RateCounter and an expvarInt
// to export counters rated at a certain interval to expvar
type ExpvarRateCounter struct {
	interval    time.Duration
	rateCounter *rc.RateCounter
	expvarInt   *expvar.Int
}

func newExpvarRateCounter(name string, interval time.Duration) *ExpvarRateCounter {
	return &ExpvarRateCounter{
		interval:    interval,
		rateCounter: rc.NewRateCounter(interval),
		expvarInt:   expvar.NewInt(name),
	}
}

// Incr increments the counter in an amount and saves exports
// it to expvar
func (c *ExpvarRateCounter) Incr(amount int64) {
	c.rateCounter.Incr(amount)
	// TODO: replace this in favor of asynchronous refresh of expavar counter
	c.expvarInt.Set(c.rateCounter.Rate())
}

// Pool holds a map of rate counters
type Pool struct {
	counters map[string]*ExpvarRateCounter
	lock     sync.RWMutex
}

var pool = &Pool{
	counters: make(map[string]*ExpvarRateCounter),
}

// FromPool returns a new counter from the pool identified by the prefix and interval
// Example: Get a counter that will hold the counts per second to `/lbcheck`
// endpoint:
//   `ratecounters.Get("endpoints.lbcheck", 1 * time.Second)`
func FromPool(counterName string, interval time.Duration) *ExpvarRateCounter {
	pool.lock.RLock()
	counter, ok := pool.counters[counterName]
	pool.lock.RUnlock()

	if !ok {
		pool.lock.Lock()
		pool.counters[counterName] = newExpvarRateCounter(counterName, interval)
		pool.lock.Unlock()

		pool.lock.RLock()
		counter = pool.counters[counterName]
		pool.lock.RUnlock()
	}

	if counter.interval != interval {
		panic(fmt.Sprintf("Counter %s already exists with a different interval", counterName))
	}

	return counter
}
