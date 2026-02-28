package base

import (
	"time"
)

// MetricHealth is a health status for a metric, and more specifically,
// when it was last checked to be "OK"
type MetricHealth struct {
	LastHealthyAt           time.Time
	SecondsSinceLastHealthy int64
}

func NewMetricHealth(lastHealthyAt time.Time) *MetricHealth {
	result := &MetricHealth{
		LastHealthyAt:           lastHealthyAt,
		SecondsSinceLastHealthy: int64(time.Since(lastHealthyAt).Seconds()),
	}
	return result
}

type MetricHealthMap map[string](*MetricHealth)

func (m MetricHealthMap) Aggregate(other MetricHealthMap) MetricHealthMap {
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
