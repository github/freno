package group

import (
	"github.com/github/freno/go/throttle"
	"github.com/outbrain/golib/log"
)

func Setup(throttler *throttle.Throttler) (ConsensusService, error) {
	consensusService, err := SetupRaft(throttler)
	if err != nil {
		return consensusService, err
	}
	if _, err := NewMySQLBackend(throttler); err != nil {
		log.Errore(err)
	}
	return consensusService, err
}
