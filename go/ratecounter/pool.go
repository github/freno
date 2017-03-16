package ratecounter

import (
	"fmt"
	"sync"
	"time"

	rc "github.com/paulbellamy/ratecounter"
)

// Pool holds a map of rate counters
type Pool struct {
	counters map[string]*rc.RateCounter
	lock     sync.RWMutex
}

var pool = &Pool{
	counters: make(map[string]*rc.RateCounter),
	lock:     sync.RWMutex{},
}

// FromPool returns a new counter from the pool identified by the prefix and interval
// Example: Get a counter that will hold the counts per second to `/lbcheck`
// endpoint:
//   `ratecounters.Get("endpoints.lbcheck", 1 * time.Second)`
func FromPool(prefix string, interval time.Duration) *rc.RateCounter {
	key := fmt.Sprintf("%s::%s", prefix, interval.String())

	pool.lock.RLock()
	counter, ok := pool.counters[key]
	pool.lock.RUnlock()

	if !ok {
		pool.lock.Lock()
		pool.counters[key] = rc.NewRateCounter(interval)
		pool.lock.Unlock()

		pool.lock.RLock()
		counter = pool.counters[key]
		pool.lock.RUnlock()
	}
	return counter
}
