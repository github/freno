package base

import (
	"time"
)

// AppThrottle is the definition for an app throtting instruction
// - Ratio: [0..1], 0 == no throttle, 1 == fully throttle
type RecentApp struct {
	CheckedAt            time.Time
	DurationSinceChecked time.Duration
}

func NewRecentApp(checkedAt time.Time) *RecentApp {
	result := &RecentApp{
		CheckedAt:            checkedAt,
		DurationSinceChecked: time.Since(checkedAt),
	}
	return result
}
