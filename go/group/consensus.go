package group

import (
	"github.com/github/freno/go/config"
	"github.com/github/freno/go/throttle"
	"github.com/outbrain/golib/log"
)

type ConsensusServiceProvider struct {
	mySQLConsensusService ConsensusService
	raftConsensusService  ConsensusService
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
	if p.raftConsensusService != nil {
		return p.raftConsensusService
	}
	if p.mySQLConsensusService != nil {
		return p.mySQLConsensusService
	}
	return nil
}

func (p *ConsensusServiceProvider) Monitor() {
	if p.raftConsensusService != nil {
		go p.raftConsensusService.Monitor()
	}
	if p.mySQLConsensusService != nil {
		go p.mySQLConsensusService.Monitor()
	}
}
