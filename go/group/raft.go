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

// ConsensusService is a freno-oriented interface for making requests that require consensus.
type ConsensusService interface {
	ThrottleApp(appName string) error
	UnthrottleApp(appName string) error
}

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

	subscribeToLeaderChanges()

	return store, nil
}

// subscribeToLeaderChanges adds a new observer to the raft setup, the observer will filter
// only those events related to leader changes, which are enqueued in a channel.
// asyncronously another goroutine sits waiting for leader changes upon which, it exports
// them into the raft.leader expvar, and some 1/0 gauges indicating who is the leader and the other
// nodes that are not the leader
func subscribeToLeaderChanges() {
	expvar.NewString("raft.leader").Set(getRaft().Leader())
	observationsChannel := make(chan raft.Observation)

	observer := raft.NewObserver(observationsChannel, false, func(o *raft.Observation) bool {
		_, isLeaderObservation := o.Data.(raft.LeaderObservation)
		return isLeaderObservation
	})
	getRaft().RegisterObserver(observer)

	go func() {
		for observation := range observationsChannel {
			leader_observation, ok := observation.Data.(raft.LeaderObservation)
			if ok {
				leader := leader_observation.Leader
				expvar.Get("raft.leader").(*expvar.String).Set(leader)
				if IsLeader() {
					metrics.GetOrRegisterGauge("raft.is_leader", nil).Update(1)
				} else {
					metrics.GetOrRegisterGauge("raft.is_leader", nil).Update(0)
				}
			}
		}
	}()
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

// Monitor is a utility function to routinely observe leadership state.
// It doesn't actually do much; merely takes notes.
func Monitor() {
	t := time.NewTicker(time.Duration(5) * time.Second)

	for {
		select {
		case <-t.C:
			leaderHint := GetLeader()
			if IsLeader() {
				leaderHint = fmt.Sprintf("%s (this host)", leaderHint)
			}
			log.Debugf("raft leader is %s", leaderHint)
		}
	}
}
