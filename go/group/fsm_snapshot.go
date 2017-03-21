package group

import (
	"encoding/json"

	"github.com/hashicorp/raft"
)

type snapshotData struct {
	throttledApps map[string]bool
}

func newSnapshotData() *snapshotData {
	return &snapshotData{
		throttledApps: make(map[string]bool),
	}
}

type fsmSnapshot struct {
	data snapshotData
}

func newFsmSnapshot() *fsmSnapshot {
	return &fsmSnapshot{
		data: *newSnapshotData(),
	}
}

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

func (f *fsmSnapshot) Release() {

}
