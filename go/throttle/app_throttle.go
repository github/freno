package throttle

import (
	"time"
)

// AppThrottle is the definition for an app throtting instruction
type AppThrottle struct {
	AppName   string
	ExpiresAt time.Time
	Ratio     float64
	Owner     string
	Reason    string
}

func NewAppThrottle(expiresAt time.Time, ratio float64) *AppThrottle {
	result := &AppThrottle{
		ExpiresAt: expiresAt,
		Ratio:     ratio,
	}
	return result
}
