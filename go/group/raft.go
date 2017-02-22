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
	peerNodes := []string{}
	for _, raftNode := range config.Settings().RaftNodes {
		nodeAddress := raftNode
		if !strings.Contains(nodeAddress, ":") {
			nodeAddress = fmt.Sprintf("%s:%d", raftNode, config.Settings().RaftListenPort)
		}
		peerNodes = append(peerNodes, nodeAddress)
	}
	bindAddress := fmt.Sprintf("127.0.0.1:%d", config.Settings().RaftListenPort)
	store = NewStore(config.Settings().RaftDataDir, bindAddress)

	if err := store.Open(peerNodes); err != nil {
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
