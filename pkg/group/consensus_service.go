package group

import (
	"time"

	"github.com/github/freno/pkg/base"
)

const monitorInterval = 5 * time.Second

type ConsensusServiceStatus struct {
	ServiceID                 string
	Healthy                   bool
	IsLeader                  bool
	Leader                    string
	State                     string
	Domain                    string
	ShareDomain               string
	ShareDomainServices       map[string]string
	ShareDomainServicesList   []string // list of this service ID and share domain services, combined
	HealthyDomainServicesList []string
}

// ConsensusService is a freno-oriented interface for making requests that require consensus.
type ConsensusService interface {
	ThrottleApp(appName string, ttlMinutes int64, expireAt time.Time, ratio float64) error
	ThrottledAppsMap() (result map[string](*base.AppThrottle))
	UnthrottleApp(appName string) error
	RecentAppsMap() (result map[string](*base.RecentApp))

	IsHealthy() bool
	IsLeader() bool
	GetLeader() string
	GetStateDescription() string
	GetSharedDomainServices() (map[string]string, error)
	GetStatus() *ConsensusServiceStatus

	Monitor()
}
