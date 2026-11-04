package config

import "testing"

func TestVitessConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		settings VitessConfigurationSettings
		empty    bool
		endpoint string
		wantErr  bool
	}{
		{name: "unset", empty: true},
		{name: "legacy HTTP", settings: VitessConfigurationSettings{API: "https://vtctld.example.com/api", Keyspace: "commerce"}, endpoint: "https://vtctld.example.com/api"},
		{name: "gRPC TLS", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:15999", Keyspace: "commerce"}, endpoint: "vtctld.example.com:15999"},
		{name: "gRPC plaintext", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:15999", GRPCInsecure: true, Keyspace: "commerce"}, endpoint: "vtctld.example.com:15999"},
		{name: "gRPC preferred", settings: VitessConfigurationSettings{API: "https://vtctld.example.com/api", GRPCAddress: "vtctld.example.com:15999", Keyspace: "commerce"}, endpoint: "vtctld.example.com:15999"},
		{name: "missing keyspace", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:15999"}, empty: true, endpoint: "vtctld.example.com:15999", wantErr: true},
		{name: "missing port", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com", Keyspace: "commerce"}, endpoint: "vtctld.example.com", wantErr: true},
		{name: "invalid port", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:65536", Keyspace: "commerce"}, endpoint: "vtctld.example.com:65536", wantErr: true},
		{name: "zero port", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:0", Keyspace: "commerce"}, endpoint: "vtctld.example.com:0", wantErr: true},
		{name: "URL is not address", settings: VitessConfigurationSettings{GRPCAddress: "https://vtctld.example.com:15999", Keyspace: "commerce"}, endpoint: "https://vtctld.example.com:15999", wantErr: true},
		{name: "missing address", settings: VitessConfigurationSettings{GRPCCAFile: "ca.pem"}, empty: true, wantErr: true},
		{name: "insecure with CA", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:15999", Keyspace: "commerce", GRPCInsecure: true, GRPCCAFile: "ca.pem"}, endpoint: "vtctld.example.com:15999", wantErr: true},
		{name: "insecure with server name", settings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:15999", Keyspace: "commerce", GRPCInsecure: true, GRPCServerName: "vtctld.example.com"}, endpoint: "vtctld.example.com:15999", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.IsEmpty(); got != tt.empty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.empty)
			}
			if got := tt.settings.Endpoint(); got != tt.endpoint {
				t.Errorf("Endpoint() = %q, want %q", got, tt.endpoint)
			}
			cluster := MySQLClusterConfigurationSettings{VitessSettings: tt.settings}
			if err := cluster.postReadAdjustments(); (err != nil) != tt.wantErr {
				t.Errorf("postReadAdjustments() = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestVitessGRPCInheritsCells(t *testing.T) {
	cluster := &MySQLClusterConfigurationSettings{
		VitessSettings: VitessConfigurationSettings{GRPCAddress: "vtctld.example.com:15999", Keyspace: "commerce"},
	}
	settings := MySQLConfigurationSettings{
		VitessCells: []string{"zone1"},
		Clusters:    map[string]*MySQLClusterConfigurationSettings{"commerce": cluster},
	}
	if err := settings.postReadAdjustments(); err != nil {
		t.Fatal(err)
	}
	if len(cluster.VitessSettings.Cells) != 1 || cluster.VitessSettings.Cells[0] != "zone1" {
		t.Fatalf("gRPC cells = %v, want inherited zone1", cluster.VitessSettings.Cells)
	}
}
