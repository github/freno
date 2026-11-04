package vitess

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/github/freno/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"vitess.io/vitess/go/vt/proto/topodata"
	"vitess.io/vitess/go/vt/proto/vtctldata"
	"vitess.io/vitess/go/vt/proto/vtctlservice"
)

type discoveryTestServer struct {
	vtctlservice.UnimplementedVtctldServer
	getTablets func(context.Context, *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error)
}

func (s *discoveryTestServer) GetTablets(ctx context.Context, req *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
	return s.getTablets(ctx, req)
}

func startDiscoveryServer(t *testing.T, handler func(context.Context, *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error), options ...grpc.ServerOption) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(options...)
	vtctlservice.RegisterVtctldServer(server, &discoveryTestServer{getTablets: handler})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-done; err != nil {
			t.Errorf("gRPC Serve: %v", err)
		}
	})
	return listener.Addr().String()
}

func replicaTablet(cell, shard string, port int32) *topodata.Tablet {
	return &topodata.Tablet{
		Alias:         &topodata.TabletAlias{Cell: cell, Uid: uint32(port)},
		Keyspace:      "commerce",
		Shard:         shard,
		Type:          topodata.TabletType_REPLICA,
		Hostname:      "tablet.example.com",
		PortMap:       map[string]int32{"grpc": 15991},
		MysqlHostname: "mysql-" + cell + ".example.com",
		MysqlPort:     port,
	}
}

func TestParseGRPCTablets(t *testing.T) {
	primary := replicaTablet("zone1", "-80", 3306)
	primary.Type = topodata.TabletType_PRIMARY
	rdonly := replicaTablet("zone1", "-80", 3306)
	rdonly.Type = topodata.TabletType_RDONLY
	otherKeyspace := replicaTablet("zone1", "-80", 3306)
	otherKeyspace.Keyspace = "other"
	response := &vtctldata.GetTabletsResponse{Tablets: []*topodata.Tablet{
		primary, rdonly, otherKeyspace,
		replicaTablet("zone1", "-80", 3307),
		replicaTablet("zone1", "80-", 3308),
		replicaTablet("zone2", "-80", 3309),
	}}
	tests := []struct {
		name  string
		shard string
		cells []string
		ports []int32
	}{
		{name: "shard and trimmed cells", shard: "-80", cells: []string{" zone1 ", "", " "}, ports: []int32{3307}},
		{name: "all shards", cells: []string{"zone1"}, ports: []int32{3307, 3308}},
		{name: "all cells", shard: "-80", cells: []string{"", " "}, ports: []int32{3307, 3309}},
		{name: "all shards and cells", ports: []int32{3307, 3308, 3309}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan *vtctldata.GetTabletsRequest, 1)
			address := startDiscoveryServer(t, func(_ context.Context, req *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
				requests <- req
				return response, nil
			})
			settings := config.VitessConfigurationSettings{
				GRPCAddress: address, GRPCInsecure: true, Keyspace: "commerce", Shard: tt.shard, Cells: tt.cells,
			}
			tablets, err := ParseTablets(settings)
			if err != nil {
				t.Fatal(err)
			}
			req := <-requests
			if req.Keyspace != "commerce" || req.Shard != tt.shard || !req.Strict || req.TabletType != topodata.TabletType_REPLICA || len(req.TabletAliases) != 0 || !reflect.DeepEqual(req.Cells, ParseCells(settings)) {
				t.Fatalf("unexpected GetTablets request: %v", req)
			}
			var ports []int32
			for _, tablet := range tablets {
				ports = append(ports, tablet.MysqlPort)
				if tablet.MysqlHostname != "mysql-"+tablet.Alias.Cell+".example.com" || tablet.Type != topodata.TabletType_REPLICA || tablet.Stats != nil {
					t.Errorf("incorrect tablet mapping: %+v", tablet)
				}
			}
			if !reflect.DeepEqual(ports, tt.ports) {
				t.Fatalf("MySQL ports = %v, want %v", ports, tt.ports)
			}
		})
	}
}

func TestParseGRPCErrorsNeverFallBack(t *testing.T) {
	var httpCalls atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpCalls.Add(1)
		json.NewEncoder(w).Encode([]Tablet{{MysqlHostname: "fallback.example.com", MysqlPort: 3306, Type: topodata.TabletType_REPLICA}})
	}))
	defer httpServer.Close()

	for _, code := range []codes.Code{codes.OK, codes.Unavailable, codes.Internal, codes.PermissionDenied, codes.Unimplemented} {
		t.Run(code.String(), func(t *testing.T) {
			address := startDiscoveryServer(t, func(_ context.Context, req *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
				if !req.Strict {
					t.Error("GetTablets must reject partial topology results")
				}
				if code == codes.OK {
					return &vtctldata.GetTabletsResponse{}, nil
				}
				return nil, status.Error(code, "topology lookup failed")
			})
			tablets, err := ParseTablets(config.VitessConfigurationSettings{
				API: httpServer.URL, GRPCAddress: address, GRPCInsecure: true, Keyspace: "commerce",
			})
			if status.Code(err) != code || len(tablets) != 0 {
				t.Fatalf("tablets = %v, error = %v; want empty result and %s", tablets, err, code)
			}
		})
	}
	if got := httpCalls.Load(); got != 0 {
		t.Fatalf("HTTP fallback called %d times", got)
	}
}

func TestParseGRPCDeadline(t *testing.T) {
	canceled := make(chan struct{})
	address := startDiscoveryServer(t, func(ctx context.Context, _ *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
		<-ctx.Done()
		close(canceled)
		return nil, status.FromContextError(ctx.Err()).Err()
	})
	start := time.Now()
	tablets, err := ParseTablets(config.VitessConfigurationSettings{
		GRPCAddress: address, GRPCInsecure: true, Keyspace: "commerce", TimeoutSecs: 1,
	})
	if status.Code(err) != codes.DeadlineExceeded || len(tablets) != 0 {
		t.Fatalf("tablets = %v, error = %v; want deadline exceeded", tablets, err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Errorf("discovery deadline took %v, want approximately 1s", elapsed)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("server context was not canceled")
	}
}

func TestParseGRPCDefaultDeadline(t *testing.T) {
	remaining := make(chan time.Duration, 1)
	address := startDiscoveryServer(t, func(ctx context.Context, _ *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, status.Error(codes.Internal, "missing deadline")
		}
		remaining <- time.Until(deadline)
		return &vtctldata.GetTabletsResponse{}, nil
	})
	tablets, err := ParseTablets(config.VitessConfigurationSettings{GRPCAddress: address, GRPCInsecure: true, Keyspace: "commerce"})
	if err != nil || len(tablets) != 0 {
		t.Fatalf("empty topology: tablets = %v, error = %v", tablets, err)
	}
	if got := <-remaining; got < 4*time.Second || got > defaultTimeout {
		t.Errorf("default deadline = %v, want approximately %v", got, defaultTimeout)
	}
}

func TestParseGRPCConnectionDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	tablets, err := ParseTablets(config.VitessConfigurationSettings{
		GRPCAddress: listener.Addr().String(), GRPCInsecure: true, Keyspace: "commerce", TimeoutSecs: 1,
	})
	if status.Code(err) != codes.DeadlineExceeded || len(tablets) != 0 {
		t.Fatalf("unresponsive listener: tablets = %v, error = %v", tablets, err)
	}
}

func TestParseGRPCMalformedReplica(t *testing.T) {
	for _, field := range []string{"alias", "hostname", "port"} {
		t.Run(field, func(t *testing.T) {
			invalid := replicaTablet("zone1", "-80", 3306)
			switch field {
			case "alias":
				invalid.Alias = nil
			case "hostname":
				invalid.MysqlHostname = ""
			case "port":
				invalid.MysqlPort = 0
			}
			address := startDiscoveryServer(t, func(context.Context, *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
				return &vtctldata.GetTabletsResponse{Tablets: []*topodata.Tablet{replicaTablet("zone1", "-80", 3307), invalid}}, nil
			})
			tablets, err := ParseTablets(config.VitessConfigurationSettings{GRPCAddress: address, GRPCInsecure: true, Keyspace: "commerce"})
			if err == nil || len(tablets) != 0 {
				t.Fatalf("malformed %s: tablets = %v, error = %v", field, tablets, err)
			}
		})
	}
}

func discoveryTLSCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vtctld.example.com"},
		DNSNames:     []string{"vtctld.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, path
}

func TestParseGRPCTLS(t *testing.T) {
	cert, caFile := discoveryTLSCertificate(t)
	invalidCAFile := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(invalidCAFile, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	address := startDiscoveryServer(t, func(context.Context, *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
		requests.Add(1)
		return &vtctldata.GetTabletsResponse{Tablets: []*topodata.Tablet{replicaTablet("zone1", "-80", 3306)}}, nil
	}, grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})))

	for _, tt := range []struct {
		name       string
		caFile     string
		serverName string
		wantErr    bool
	}{
		{name: "verified CA and name", caFile: caFile, serverName: "vtctld.example.com"},
		{name: "untrusted CA", serverName: "vtctld.example.com", wantErr: true},
		{name: "wrong server name", caFile: caFile, serverName: "wrong.example.com", wantErr: true},
		{name: "address does not match certificate", caFile: caFile, wantErr: true},
		{name: "missing CA file", caFile: filepath.Join(t.TempDir(), "missing.pem"), wantErr: true},
		{name: "invalid CA file", caFile: invalidCAFile, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tablets, err := ParseTablets(config.VitessConfigurationSettings{
				GRPCAddress: address, GRPCCAFile: tt.caFile, GRPCServerName: tt.serverName, Keyspace: "commerce", TimeoutSecs: 1,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("TLS discovery error = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr && len(tablets) != 0 {
				t.Errorf("failed TLS returned tablets: %v", tablets)
			}
		})
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server received %d requests, want only the verified request", got)
	}
}

type discoveryConnectionStats struct {
	closed chan struct{}
}

func (s *discoveryConnectionStats) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (s *discoveryConnectionStats) HandleRPC(context.Context, stats.RPCStats) {}

func (s *discoveryConnectionStats) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (s *discoveryConnectionStats) HandleConn(_ context.Context, event stats.ConnStats) {
	if _, ok := event.(*stats.ConnEnd); ok {
		s.closed <- struct{}{}
	}
}

func TestParseGRPCClosesConnections(t *testing.T) {
	connectionStats := &discoveryConnectionStats{closed: make(chan struct{}, 2)}
	address := startDiscoveryServer(t, func(context.Context, *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
		return &vtctldata.GetTabletsResponse{}, nil
	}, grpc.StatsHandler(connectionStats))
	for i := 0; i < 2; i++ {
		_, err := ParseTablets(config.VitessConfigurationSettings{GRPCAddress: address, GRPCInsecure: true, Keyspace: "commerce"})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-connectionStats.closed:
		case <-time.After(time.Second):
			t.Fatal("discovery did not close its gRPC connection")
		}
	}
}

func TestDiscoveryConcurrentTransports(t *testing.T) {
	grpcAddress := startDiscoveryServer(t, func(context.Context, *vtctldata.GetTabletsRequest) (*vtctldata.GetTabletsResponse, error) {
		return &vtctldata.GetTabletsResponse{Tablets: []*topodata.Tablet{replicaTablet("zone1", "-80", 3306)}}, nil
	})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]Tablet{{MysqlHostname: "mysql.example.com", MysqlPort: 3306, Type: topodata.TabletType_REPLICA}})
	}))
	t.Cleanup(httpServer.Close)
	t.Run("parallel calls", func(t *testing.T) {
		for i := 0; i < 12; i++ {
			i := i
			t.Run(string(rune('a'+i)), func(t *testing.T) {
				t.Parallel()
				settings := config.VitessConfigurationSettings{API: httpServer.URL, Keyspace: "commerce", TimeoutSecs: uint(i % 3)}
				if i%2 == 0 {
					settings.GRPCAddress = grpcAddress
					settings.GRPCInsecure = true
				}
				tablets, err := ParseTablets(settings)
				if err != nil || len(tablets) != 1 {
					t.Fatalf("concurrent discovery: tablets = %v, error = %v", tablets, err)
				}
			})
		}
	})
}
