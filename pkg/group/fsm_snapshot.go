package group

import (
	"encoding/json"

	"github.com/github/freno/pkg/base"

	"github.com/github/freno/internal/raft"
)

// snapshotData holds whatever data we wish to persist as part of raft snapshotting
// it will mostly duplicate data stored in `throttler`.
type snapshotData struct {
	throttledApps map[string](base.AppThrottle)
}

func newSnapshotData() *snapshotData {
	return &snapshotData{
		throttledApps: make(map[string](base.AppThrottle)),
	}
}

// fsmSnapshot handles raft persisting of snapshots
type fsmSnapshot struct {
	data snapshotData
}

func newFsmSnapshot() *fsmSnapshot {
	return &fsmSnapshot{
		data: *newSnapshotData(),
	}
}

// Persist
func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// Encode data.
		b, err := json.Marshal(f.data)
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

// Release
func (f *fsmSnapshot) Release() {
}
