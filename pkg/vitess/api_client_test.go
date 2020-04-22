package vitess

import (
	"testing"

	"vitess.io/vitess/go/vt/proto/topodata"
)

func TestFilterReplicaTablets(t *testing.T) {
	replicas := filterReplicaTablets([]Tablet{
		{
			MysqlHostname: t.Name() + "1",
			Type:          topodata.TabletType_MASTER,
		},
		{
			MysqlHostname: t.Name() + "2", // this node is valid
			Type:          topodata.TabletType_REPLICA,
		},
		{
			MysqlHostname: t.Name() + "3",
			Type:          topodata.TabletType_SPARE,
		},
		{
			MysqlHostname: t.Name() + "4",
			Type:          topodata.TabletType_BACKUP,
		},
		{

			MysqlHostname: t.Name() + "5",
			Type:          topodata.TabletType_RESTORE,
		},
	})
	if len(replicas) != 1 {
		t.Fatalf("Expected 1 replica, got %v", replicas)
	}
	if replicas[0].MysqlHostname != t.Name()+"2" {
		t.Fatalf("Expected hostname %q, got %q", t.Name()+"2", replicas[0].MysqlHostname)
	}
}
