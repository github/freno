package group

import (
	"encoding/json"
	"io"
	"time"

	"github.com/github/freno/internal/raft"
	"github.com/outbrain/golib/log"
)

// fsm is a raft finite state machine, that is freno aware. It applies events/commands
// onto the freno throttler.
type fsm Store

// Apply applies a Raft log entry to the key-value store.
func (f *fsm) Apply(l *raft.Log) interface{} {
	var c command
	if err := json.Unmarshal(l.Data, &c); err != nil {
		log.Errorf("failed to unmarshal command: %s", err.Error())
	}

	log.Debugf("freno/raft: applying command: %+v", c)
	switch c.Operation {
	case "throttle":
		return f.applyThrottleApp(c.Key, c.ExpireAt, c.Ratio)
	case "unthrottle":
		return f.applyUnthrottleApp(c.Key)
	}
	return log.Errorf("unrecognized command operation: %s", c.Operation)
}

// Snapshot returns a snapshot object of freno's state
func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	log.Debugf("freno/raft: creating snapshot")
	snapshot := newFsmSnapshot()

	for appName, appThrottle := range f.throttler.ThrottledAppsMap() {
		snapshot.data.throttledApps[appName] = *appThrottle
	}
	return snapshot, nil
}

// Restore restores freno state
func (f *fsm) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	data := newSnapshotData()
	if err := json.NewDecoder(rc).Decode(&data); err != nil {
		return err
	}
	for appName, appThrottle := range data.throttledApps {
		f.throttler.ThrottleApp(appName, appThrottle.ExpireAt, appThrottle.Ratio)
	}
	log.Debugf("freno/raft: restored from snapshot: %d elements restored", len(data.throttledApps))
	return nil
}

// applyThrottleApp will apply a "throttle" command locally (this applies as result of the raft consensus algorithm)
func (f *fsm) applyThrottleApp(appName string, expireAt time.Time, ratio float64) interface{} {
	f.throttler.ThrottleApp(appName, expireAt, ratio)
	return nil
}

// applyThrottleApp will apply a "unthrottle" command locally (this applies as result of the raft consensus algorithm)
func (f *fsm) applyUnthrottleApp(appName string) interface{} {
	f.throttler.UnthrottleApp(appName)
	return nil
}
