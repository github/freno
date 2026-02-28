//
// Provide distributed consensus services.
// Underlying implementation is Raft, via https://godoc.org/github.com/hashicorp/raft
//
// This file provides generic access functions to setup & check group communication.
//

package group

import (
	"fmt"
	"strings"

	"github.com/github/freno/pkg/config"
	"github.com/github/freno/pkg/throttle"
	"github.com/outbrain/golib/log"

	"github.com/github/freno/internal/raft"
)

const RaftDBFile = "freno-raft.db"

var store *Store

// Setup creates the entire raft shananga. Creates the store, associates with the throttler,
// contacts peer nodes, and subscribes to leader changes to export them.
func SetupRaft(throttler *throttle.Throttler) (ConsensusService, error) {
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
