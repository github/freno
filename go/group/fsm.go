package group

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/raft"
	"github.com/outbrain/golib/log"
)

type fsm Store

// Apply applies a Raft log entry to the key-value store.
func (f *fsm) Apply(l *raft.Log) interface{} {
	var c command
	if err := json.Unmarshal(l.Data, &c); err != nil {
		panic(fmt.Sprintf("failed to unmarshal command: %s", err.Error()))
	}

	log.Debugf("freno/raft: applying command: %+v", c)
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
	log.Debugf("freno/raft: creating snapshot")
	snapshot := newFsmSnapshot()

	for appName, _ := range f.throttler.ThrottledAppsSnapshot() {
		snapshot.data.throttledApps[appName] = true
	}
	return snapshot, nil
}

// Restore stores the key-value store to a previous state.
func (f *fsm) Restore(rc io.ReadCloser) error {
	data := newSnapshotData()
	if err := json.NewDecoder(rc).Decode(&data); err != nil {
		return err
	}
	for appName := range data.throttledApps {
		f.throttler.ThrottleApp(appName)
	}
	log.Debugf("freno/raft: restored from snapshot: %d elements restored", len(data.throttledApps))
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
