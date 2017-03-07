package ratecounter

import (
	"fmt"
	"sync"
	"time"

	rc "github.com/paulbellamy/ratecounter"
)

type Pool struct {
	counters map[string]*rc.RateCounter
	lock     sync.Mutex
}

var pool = &Pool{
	counters: make(map[string]*rc.RateCounter),
}

// FromPool returns a new counter from the pool identified by the prefix and interval
// Example: Get a counter that will hold the counts per second to `/lbcheck`
// endpoint:
//   `ratecounters.Get("endpoints.lbcheck", 1 * time.Second)`
func FromPool(prefix string, interval time.Duration) *rc.RateCounter {
	key := fmt.Sprintf("%s::%s", prefix, interval.String())

	pool.lock.Lock()
	defer pool.lock.Unlock()
	counter, ok := pool.counters[key]
	if !ok {
		pool.counters[key] = rc.NewRateCounter(interval)
		counter = pool.counters[key]
	}
	return counter
}
