package base

import (
	"time"
)

// AppThrottle is the definition for an app throtting instruction
// - Ratio: [0..1], 0 == no throttle, 1 == fully throttle
type AppThrottle struct {
	ExpireAt time.Time
	Ratio    float64
}

func NewAppThrottle(expireAt time.Time, ratio float64) *AppThrottle {
	result := &AppThrottle{
		ExpireAt: expireAt,
		Ratio:    ratio,
	}
	return result
}

// DisplayAppThrottle is a type for displaying data back to the end 
// user via chatop. Handles infinite TTL and allows ExpiresAt to be "INFINITE"
type DisplayAppThrottle struct {
	ExpireAt string
	Ratio    float64
}