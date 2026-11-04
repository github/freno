package vitess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/github/freno/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"vitess.io/vitess/go/vt/proto/topodata"
	"vitess.io/vitess/go/vt/proto/vtctldata"
	"vitess.io/vitess/go/vt/proto/vtctlservice"
)

func discoveryCredentials(settings config.VitessConfigurationSettings) (credentials.TransportCredentials, error) {
	if settings.GRPCInsecure {
		return insecure.NewCredentials(), nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: settings.GRPCServerName,
	}
	if settings.GRPCCAFile != "" {
		pem, err := os.ReadFile(settings.GRPCCAFile)
		if err != nil {
			return nil, fmt.Errorf("read Vitess gRPC CA: %w", err)
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		if !tlsConfig.RootCAs.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("Vitess gRPC CA file contains no certificates")
		}
	}
	return credentials.NewTLS(tlsConfig), nil
}

func parseGRPCTablets(settings config.VitessConfigurationSettings) ([]Tablet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout(settings))
	defer cancel()

	creds, err := discoveryCredentials(settings)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(settings.GRPCAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("connect to Vitess gRPC: %w", err)
	}
	defer conn.Close()

	response, err := vtctlservice.NewVtctldClient(conn).GetTablets(ctx, &vtctldata.GetTabletsRequest{
		Keyspace:   settings.Keyspace,
		Shard:      settings.Shard,
		Cells:      ParseCells(settings),
		Strict:     true,
		TabletType: topodata.TabletType_REPLICA,
	})
	if err != nil {
		return nil, fmt.Errorf("Vitess GetTablets: %w", err)
	}

	validCells := ParseCells(settings)
	var tablets []Tablet
	for _, entry := range response.GetTablets() {
		if entry == nil {
			return nil, fmt.Errorf("Vitess GetTablets returned a nil tablet")
		}
		if entry.Keyspace != settings.Keyspace || (settings.Shard != "" && entry.Shard != settings.Shard) {
			continue
		}
		tablet := Tablet{
			Alias:         entry.Alias,
			MysqlHostname: entry.MysqlHostname,
			MysqlPort:     entry.MysqlPort,
			Type:          entry.Type,
		}
		if tablet.Type != topodata.TabletType_REPLICA {
			continue
		}
		if tablet.Alias == nil {
			return nil, fmt.Errorf("Vitess GetTablets returned a replica without an alias")
		}
		if !tablet.HasValidCell(validCells) {
			continue
		}
		if tablet.MysqlHostname == "" || tablet.MysqlPort < 1 || tablet.MysqlPort > 65535 {
			return nil, fmt.Errorf("Vitess GetTablets returned a replica without a valid MySQL address")
		}
		tablets = append(tablets, tablet)
	}
	return tablets, nil
}
