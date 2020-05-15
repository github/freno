package base

import (
	"time"
)

// StoreHealth is a health status for a metric, and more specifically,
// when it was last checked to be "OK"
type StoreHealth struct {
	LastHealthyAt           time.Time
	SecondsSinceLastHealthy int64
}

func NewStoreHealth(lastHealthyAt time.Time) *StoreHealth {
	result := &StoreHealth{
		LastHealthyAt:           lastHealthyAt,
		SecondsSinceLastHealthy: int64(time.Since(lastHealthyAt).Seconds()),
	}
	return result
}

type StoreHealthMap map[string](*StoreHealth)

func (m StoreHealthMap) Aggregate(other StoreHealthMap) StoreHealthMap {
	for metricName, otherHealth := range other {
		if currentHealth, ok := m[metricName]; ok {
			if currentHealth.SecondsSinceLastHealthy < otherHealth.SecondsSinceLastHealthy {
				m[metricName] = otherHealth
			}
		} else {
			m[metricName] = otherHealth
		}
	}
	return m
}
