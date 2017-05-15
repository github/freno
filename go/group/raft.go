//
// Provide distributed consensus services.
// Underlying implementation is Raft, via https://godoc.org/github.com/hashicorp/raft
//
// This file provides generic access functions to setup & check group communication.
//

package group

import (
	"expvar"
	"fmt"
	"strings"
	"time"

	"github.com/github/freno/go/config"
	"github.com/github/freno/go/throttle"
	"github.com/outbrain/golib/log"
	metrics "github.com/rcrowley/go-metrics"

	"github.com/hashicorp/raft"
)

const RaftDBFile = "freno-raft.db"

var store *Store

// Setup creates the entire raft shananga. Creates the store, associates with the throttler,
// contacts peer nodes, and subscribes to leader changes to export them.
func Setup(throttler *throttle.Throttler) (ConsensusService, error) {
	store = NewStore(config.Settings().RaftDataDir, normalizeRaftNode(config.Settings().RaftBind), throttler)

	peerNodes := []string{}
	for _, raftNode := range config.Settings().RaftNodes {
		peerNodes = append(peerNodes, normalizeRaftNode(raftNode))
	}
	if err := store.Open(peerNodes); err != nil {
		return nil, log.Errorf("failed to open raft store: %s", err.Error())
	}

	return store, nil
}

// getRaft is a convenience method
func getRaft() *raft.Raft {
	return store.raft
}

// normalizeRaftNode attempts to make sure there's a port to the given node.
// It consults the DefaultRaftPort when there isn't
func normalizeRaftNode(node string) string {
	if strings.Contains(node, ":") {
		return node
	}
	if config.Settings().DefaultRaftPort == 0 {
		return node
	}
	node = fmt.Sprintf("%s:%d", node, config.Settings().DefaultRaftPort)
	return node
}

// IsLeader tells if this node is the current raft leader
func IsLeader() bool {
	future := getRaft().VerifyLeader()
	err := future.Error()
	return err == nil
}

// GetLeader returns identity of raft leader
func GetLeader() string {
	return getRaft().Leader()
}

// GetState returns current raft state
func GetState() raft.RaftState {
	return getRaft().State()
}

// Monitor is a utility function to routinely observe leadership state.
// It doesn't actually do much; merely takes notes.
func Monitor() {
	t := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-t.C:
			leaderHint := GetLeader()

			leaderExpVar := expvar.Get("raft.leader")
			if leaderExpVar == nil {
				leaderExpVar = expvar.NewString("raft.leader")
			}
			leaderExpVar.(*expvar.String).Set(leaderHint)

			if IsLeader() {
				leaderHint = fmt.Sprintf("%s (this host)", leaderHint)
				metrics.GetOrRegisterGauge("raft.is_leader", nil).Update(1)
			} else {
				metrics.GetOrRegisterGauge("raft.is_leader", nil).Update(0)
			}
			log.Debugf("raft leader is %s; state: %s", leaderHint, GetState().String())
		}
	}
}
