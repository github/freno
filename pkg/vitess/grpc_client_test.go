package vitess

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/github/freno/pkg/config"
	"google.golang.org/grpc"
	"vitess.io/vitess/go/vt/proto/topodata"
	"vitess.io/vitess/go/vt/proto/vtctldata"
	"vitess.io/vitess/go/vt/proto/vtctlservice"
)

type fakeVtctld struct {
	vtctlservice.UnimplementedVtctldServer
	mu      sync.Mutex
	request *vtctldata.GetTabletsRequest
}

func (vtctld *fakeVtctld) GetTablets(_ context.Context, request *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
	vtctld.mu.Lock()
	vtctld.request = request
	vtctld.mu.Unlock()
	return &vtctldata.GetTabletsResponse{Tablets: []*topodata.Tablet{
		{Alias: &topodata.TabletAlias{Cell: "cell1"}, Shard: "-80", MysqlHostname: "replica1", MysqlPort: 3306, Type: topodata.TabletType_REPLICA},
		{Alias: &topodata.TabletAlias{Cell: "cell2"}, Shard: "-80", MysqlHostname: "replica2", MysqlPort: 3306, Type: topodata.TabletType_REPLICA},
		{Alias: &topodata.TabletAlias{Cell: "cell2"}, Shard: "80-", MysqlHostname: "replica3", MysqlPort: 3306, Type: topodata.TabletType_REPLICA},
		{Alias: &topodata.TabletAlias{Cell: "cell2"}, Shard: "-80", MysqlHostname: "primary", MysqlPort: 3306, Type: topodata.TabletType_PRIMARY},
	}}, nil
}

func (vtctld *fakeVtctld) lastRequest() *vtctldata.GetTabletsRequest {
	vtctld.mu.Lock()
	defer vtctld.mu.Unlock()
	return vtctld.request
}

func TestParseTabletsGRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	vtctld := &fakeVtctld{}
	server := grpc.NewServer()
	vtctlservice.RegisterVtctldServer(server, vtctld)
	go server.Serve(listener)
	defer server.Stop()

	t.Run("shard-and-cells", func(t *testing.T) {
		tablets, err := ParseTablets(config.VitessConfigurationSettings{
			GRPCAddress:  listener.Addr().String(),
			GRPCInsecure: true,
			Cells:        []string{"cell2"},
			Keyspace:     "test",
			Shard:        "-80",
		})
		if err != nil {
			t.Fatalf("Expected no error, got %q", err)
		}
		if len(tablets) != 1 || tablets[0].MysqlHostname != "replica2" || tablets[0].MysqlPort != 3306 {
			t.Fatalf("Expected only replica2:3306, got %+v", tablets)
		}

		// vtctld ignores Cells when Shard is set, so the request must be cell-scoped and the shard filtered locally
		request := vtctld.lastRequest()
		if request.GetKeyspace() != "test" || request.GetShard() != "" || len(request.GetCells()) != 1 || request.GetCells()[0] != "cell2" {
			t.Fatalf("Unexpected GetTablets request: %+v", request)
		}
	})

	t.Run("shard", func(t *testing.T) {
		tablets, err := ParseTablets(config.VitessConfigurationSettings{
			GRPCAddress:  listener.Addr().String(),
			GRPCInsecure: true,
			Keyspace:     "test",
			Shard:        "-80",
		})
		if err != nil {
			t.Fatalf("Expected no error, got %q", err)
		}
		if len(tablets) != 2 || tablets[0].MysqlHostname != "replica1" || tablets[1].MysqlHostname != "replica2" {
			t.Fatalf("Expected replica1 and replica2, got %+v", tablets)
		}

		request := vtctld.lastRequest()
		if request.GetKeyspace() != "test" || request.GetShard() != "-80" || len(request.GetCells()) != 0 {
			t.Fatalf("Unexpected GetTablets request: %+v", request)
		}
	})

	t.Run("insecure-required", func(t *testing.T) {
		_, err := ParseTablets(config.VitessConfigurationSettings{
			GRPCAddress: listener.Addr().String(),
			Keyspace:    "test",
		})
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}
