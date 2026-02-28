package group

import (
	"time"

	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/throttle"
	"github.com/outbrain/golib/log"
	metrics "github.com/rcrowley/go-metrics"
)

var ForceLeadership = false

type ConsensusServiceProvider struct {
	mySQLConsensusService ConsensusService
	raftConsensusService  ConsensusService
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func NewConsensusServiceProvider(throttler *throttle.Throttler) (p *ConsensusServiceProvider, err error) {
	p = &ConsensusServiceProvider{}

	if config.Settings().RaftDataDir != "" {
		if p.raftConsensusService, err = SetupRaft(throttler); err != nil {
			log.Errore(err)
		}
	}
	if config.Settings().BackendMySQLHost != "" {
		if p.mySQLConsensusService, err = NewMySQLBackend(throttler); err != nil {
			log.Errore(err)
		}
	}
	if p.raftConsensusService == nil && p.mySQLConsensusService == nil {
		return nil, log.Errorf("Could not create any consensus service")
	}
	return p, nil
}

func (p *ConsensusServiceProvider) GetConsensusService() ConsensusService {
	if p.mySQLConsensusService != nil {
		return p.mySQLConsensusService
	}
	if p.raftConsensusService != nil {
		return p.raftConsensusService
	}
	return nil
}

func (p *ConsensusServiceProvider) Monitor() {
	if p.mySQLConsensusService != nil {
		go p.mySQLConsensusService.Monitor()
	}
	if p.raftConsensusService != nil {
		go p.raftConsensusService.Monitor()
	}

	t := time.NewTicker(monitorInterval)
	s := p.GetConsensusService()
	if s == nil {
		return
	}
	for range t.C {
		leaderState := boolToInt64(s.IsLeader())
		go metrics.GetOrRegisterGauge("consensus.is_leader", nil).Update(leaderState)

		healthState := boolToInt64(s.IsHealthy())
		go metrics.GetOrRegisterGauge("consensus.is_healthy", nil).Update(healthState)
	}
}
