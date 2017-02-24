//
// Provide distributed concensus services.
// Underlying implementation is Raft, via https://godoc.org/github.com/hashicorp/raft
//
// This file provides generic access functions to setup & check group communication.
//

package group

import (
	"time"

	"github.com/github/freno/go/config"
	"github.com/outbrain/golib/log"

	"github.com/hashicorp/raft"
)

const RaftDBFile = "freno-raft.db"

var store *Store

func Setup() error {
	store = NewStore(config.Settings().RaftDataDir, config.Settings().RaftBind)

	if err := store.Open(config.Settings().RaftNodes); err != nil {
		return log.Errorf("failed to open raft store: %s", err.Error())
	}

	return nil
}

func getRaft() *raft.Raft {
	return store.raft
}

func IsLeader() bool {
	future := getRaft().VerifyLeader()
	err := future.Error()
	return err == nil
}

func GetLeader() string {
	return getRaft().Leader()
}

func Monitor() {
	t := time.NewTicker(time.Duration(1) * time.Second)

	for {
		select {
		case <-t.C:
			log.Debugf("raft: leader is %+v", getRaft().Leader())
			if IsLeader() {
				log.Debugf("I'm the leader")
			}
		}
	}
}
