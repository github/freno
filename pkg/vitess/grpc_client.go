package vitess

import (
	"context"
	"fmt"
	"time"

	"github.com/github/freno/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"vitess.io/vitess/go/vt/proto/vtctldata"
	"vitess.io/vitess/go/vt/proto/vtctlservice"
)

// parseGRPCTablets is the plaintext gRPC equivalent of ParseTablets' HTTP API request, using vtctld's GetTablets
func parseGRPCTablets(settings config.VitessConfigurationSettings) (tablets []Tablet, err error) {
	if !settings.GRPCInsecure {
		return tablets, fmt.Errorf("VitessSettings.GRPCInsecure must be true: only plaintext gRPC is supported")
	}
	timeout := defaultTimeout
	if settings.TimeoutSecs != 0 {
		timeout = time.Duration(settings.TimeoutSecs) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(settings.GRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return tablets, err
	}
	defer conn.Close()

	request := &vtctldata.GetTabletsRequest{
		Keyspace: settings.Keyspace,
		Shard:    settings.Shard,
		Cells:    ParseCells(settings),
	}
	if len(request.Cells) > 0 {
		// GetTablets ignores Cells when Shard is set, so query only the cells and filter the shard below
		request.Shard = ""
	}
	resp, err := vtctlservice.NewVtctldClient(conn).GetTablets(ctx, request)
	if err != nil {
		return tablets, err
	}
	for _, tablet := range resp.GetTablets() {
		if settings.Shard != "" && tablet.GetShard() != settings.Shard {
			continue
		}
		tablets = append(tablets, Tablet{
			Alias:         tablet.GetAlias(),
			MysqlHostname: tablet.GetMysqlHostname(),
			MysqlPort:     tablet.GetMysqlPort(),
			Type:          tablet.GetType(),
		})
	}
	return filterReplicaTablets(settings, tablets), nil
}
