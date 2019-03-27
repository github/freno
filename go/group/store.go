//
// Raft implementation
//
// This file is based on https://github.com/otoolep/hraftd

package group

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/github/freno/go/base"
	"github.com/github/freno/go/throttle"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/outbrain/golib/log"
)

const (
	retainSnapshotCount = 2
	raftTimeout         = 10 * time.Second
)

// command struct is the data type we move around as raft events. We can easily model all
// our events using op/key/value setup.
type command struct {
	Operation string    `json:"op,omitempty"`
	Key       string    `json:"key,omitempty"`
	Value     string    `json:"value,omitempty"`
	ExpireAt  time.Time `json:"expire,omitempty"`
	Ratio     float64   `json:"ratio,omitempty"`
}

// The store is a raft store that is freno-aware.
// It operates on a `throttler` instance on given events/commands.
// Store implements consensusService, which is a freno-oriented interface for
// running operations via consensus.
type Store struct {
	raftDir  string
	raftBind string

	throttler *throttle.Throttler

	raft *raft.Raft // The consensus mechanism
}

// NewStore inits and returns a new store
func NewStore(raftDir string, raftBind string, throttler *throttle.Throttler) *Store {
	return &Store{
		raftDir:   raftDir,
		raftBind:  raftBind,
		throttler: throttler,
	}
}

// Open opens the store. If enableSingle is set, and there are no existing peers,
// then this node becomes the first node, and therefore leader, of the cluster.
func (store *Store) Open(peerNodes []string) error {
	// Setup Raft configuration.
	config := raft.DefaultConfig()

	// Setup Raft communication.
	addr, err := net.ResolveTCPAddr("tcp", store.raftBind)
	if err != nil {
		return err
	}
	transport, err := raft.NewTCPTransport(store.raftBind, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return err
	}

	peers := make([]string, 0, 10)
	for _, peerNode := range peerNodes {
		peerNode = strings.TrimSpace(peerNode)
		peers = raft.AddUniquePeer(peers, peerNode)
	}

	// Create peer storage.
	peerStore := &raft.StaticPeers{}
	if err := peerStore.SetPeers(peers); err != nil {
		return err
	}

	// Allow the node to enter single-mode, potentially electing itself, if
	// explicitly enabled and there is only 1 node in the cluster already.
	if len(peerNodes) == 0 && len(peers) <= 1 {
		log.Infof("enabling single-node mode")
		config.EnableSingleNode = true
		config.DisableBootstrapAfterElect = false
	}

	// Create the snapshot store. This allows the Raft to truncate the log.
	snapshots, err := raft.NewFileSnapshotStore(store.raftDir, retainSnapshotCount, os.Stderr)
	if err != nil {
		return fmt.Errorf("file snapshot store: %s", err)
	}

	// Create the log store and stable store.
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(store.raftDir, RaftDBFile))
	if err != nil {
		return fmt.Errorf("error creating new bolt store: %s", err)
	}
	log.Infof("bolt store created")

	// Instantiate the Raft systems.
	if store.raft, err = raft.NewRaft(config, (*fsm)(store), logStore, logStore, snapshots, peerStore, transport); err != nil {
		return fmt.Errorf("error creating new raft: %s", err)
	}
	log.Infof("new raft created")

	return nil
}

// genericCommand requests consensus for applying a single command.
func (store *Store) genericCommand(c *command) error {
	if store.raft.State() != raft.Leader {
		return fmt.Errorf("not leader")
	}

	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	f := store.raft.Apply(b, raftTimeout)
	return f.Error()
}

// ThrottleApp, as implied by consensusService, is a raft oepration request which
// will ask for consensus.
func (store *Store) ThrottleApp(appName string, ttlMinutes int64, expireAt time.Time, ratio float64) error {
	c := &command{
		Operation: "throttle",
		Key:       appName,
		ExpireAt:  expireAt,
		Ratio:     ratio,
	}
	return store.genericCommand(c)
}

// UnthrottleApp, as implied by consensusService, is a raft oepration request which
// will ask for consensus.
func (store *Store) UnthrottleApp(appName string) error {
	c := &command{
		Operation: "unthrottle",
		Key:       appName,
	}
	return store.genericCommand(c)
}

func (store *Store) ThrottledAppsMap() (result map[string](*base.AppThrottle)) {
	return store.throttler.ThrottledAppsMap()
}

func (store *Store) RecentAppsMap() (result map[string](*base.RecentApp)) {
	return store.throttler.RecentAppsMap()
}

// Join joins a node, located at addr, to this store. The node must be ready to
// respond to Raft communications at that address.
func (store *Store) Join(addr string) error {
	log.Infof("received join request for remote node as %s", addr)

	f := store.raft.AddPeer(addr)
	if f.Error() != nil {
		return f.Error()
	}
	log.Infof("node at %s joined successfully", addr)
	return nil
}
