package base

import (
	"time"
)

// AppThrottle is the definition for an app throtting instruction
// - Ratio: [0..1], 0 == no throttle, 1 == fully throttle
type RecentApp struct {
	CheckedAtEpoch      int64
	MinutesSinceChecked int64
}

func NewRecentApp(checkedAt time.Time) *RecentApp {
	result := &RecentApp{
		CheckedAtEpoch:      checkedAt.Unix(),
		MinutesSinceChecked: int64(time.Since(checkedAt).Minutes()),
	}
	return result
}
