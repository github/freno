//
// Raft implementation
//
// This file is based on https://github.com/otoolep/hraftd

package group

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/github/freno/go/throttle"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/outbrain/golib/log"
)

const (
	retainSnapshotCount = 2
	raftTimeout         = 10 * time.Second
)

type command struct {
	Operation string `json:"op,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
}

// Store is a simple key-value store, where all changes are made via Raft consensus.
type Store struct {
	raftDir  string
	raftBind string

	throttler *throttle.Throttler

	raft *raft.Raft // The consensus mechanism
}

// New returns a new Store.
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

	// Allow the node to entry single-mode, potentially electing itself, if
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

func (store *Store) ThrottleApp(appName string) error {
	c := &command{
		Operation: "throttle",
		Key:       appName,
	}
	return store.genericCommand(c)
}

func (store *Store) UnthrottleApp(appName string) error {
	c := &command{
		Operation: "unthrottle",
		Key:       appName,
	}
	return store.genericCommand(c)
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

type fsm Store

// Apply applies a Raft log entry to the key-value store.
func (f *fsm) Apply(l *raft.Log) interface{} {
	var c command
	if err := json.Unmarshal(l.Data, &c); err != nil {
		panic(fmt.Sprintf("failed to unmarshal command: %s", err.Error()))
	}

	log.Debugf("Applying via raft %+v", c)
	switch c.Operation {
	case "throttle":
		return f.applyThrottleApp(c.Key)
	case "unthrottle":
		return f.applyUnthrottleApp(c.Key)
	default:
		panic(fmt.Sprintf("unrecognized command operation: %s", c.Operation))
	}
}

// Snapshot returns a snapshot of the key-value store.
func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	return nil, nil
	// // Clone the map.
	// o := make(map[string]string)
	// for k, v := range f.m {
	// 	o[k] = v
	// }
	// return &fsmSnapshot{store: o}, nil
}

// Restore stores the key-value store to a previous state.
func (f *fsm) Restore(rc io.ReadCloser) error {
	// o := make(map[string]string)
	// if err := json.NewDecoder(rc).Decode(&o); err != nil {
	// 	return err
	// }
	//
	// // Set the state from the snapshot, no lock required according to
	// // Hashicorp docs.
	// f.m = o
	return nil
}

func (f *fsm) applyThrottleApp(appName string) interface{} {
	f.throttler.ThrottleApp(appName)
	return nil
}

func (f *fsm) applyUnthrottleApp(appName string) interface{} {
	f.throttler.UnthrottleApp(appName)
	return nil
}

type fsmSnapshot struct {
	store map[string]string
}

func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// Encode data.
		b, err := json.Marshal(f.store)
		if err != nil {
			return err
		}

		// Write data to sink.
		if _, err := sink.Write(b); err != nil {
			return err
		}

		// Close the sink.
		if err := sink.Close(); err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		sink.Cancel()
		return err
	}

	return nil
}

func (f *fsmSnapshot) Release() {}
