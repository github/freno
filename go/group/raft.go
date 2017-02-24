//
// Provide distributed concensus services.
// Underlying implementation is Raft, via https://godoc.org/github.com/hashicorp/raft
//
// This file provides generic access functions to setup & check group communication.
//

package group

import (
	"fmt"
	"strings"
	"time"

	"github.com/github/freno/go/config"
	"github.com/outbrain/golib/log"

	"github.com/hashicorp/raft"
)

const RaftDBFile = "freno-raft.db"

var store *Store

func Setup() error {
	store = NewStore(config.Settings().RaftDataDir, normalizeRaftNode(config.Settings().RaftBind))

	peerNodes := []string{}
	for _, raftNode := range config.Settings().RaftNodes {
		peerNodes = append(peerNodes, normalizeRaftNode(raftNode))
	}
	if err := store.Open(peerNodes); err != nil {
		return log.Errorf("failed to open raft store: %s", err.Error())
	}

	return nil
}

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
