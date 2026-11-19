package haproxy

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseCsvHostsWithServerAddresses(t *testing.T) {
	for _, tt := range []struct {
		address  string
		hostname string
		port     int
	}{
		{"192.0.2.10:3307", "192.0.2.10", 3307},
		{"[2001:db8::10]:3308", "2001:db8::10", 3308},
		{"db.example.com:3309", "db.example.com", 3309},
	} {
		t.Run(tt.address, func(t *testing.T) {
			csv := "# pxname,svname,status,addr\n" +
				fmt.Sprintf("writers,primary,no check,%s\n", tt.address) +
				"writers,BACKEND,UP,\nwriters,FRONTEND,OPEN,\n"
			hosts, err := ParseCsvHostsWithServerAddresses(csv, "writers")
			if err != nil {
				t.Fatal(err)
			}
			if len(hosts) != 1 || hosts[0].Hostname != tt.hostname || hosts[0].Port != tt.port {
				t.Fatalf("hosts = %+v, want %s port %d", hosts, tt.hostname, tt.port)
			}
		})
	}
}

func TestParseCsvHostsWithServerAddressesRejectsInvalidAddresses(t *testing.T) {
	for _, address := range []string{
		"", "192.0.2.10", ":3306", "192.0.2.10:", "192.0.2.10:mysql",
		"192.0.2.10:0", "192.0.2.10:-1", "192.0.2.10:65536",
		"2001:db8::10:3306", "bad host:3306",
		"0.0.0.0:3306", "[::]:3306",
	} {
		t.Run(address, func(t *testing.T) {
			csv := "# pxname,svname,status,addr\n" +
				"writers,healthy,UP,192.0.2.11:3306\n" +
				fmt.Sprintf("writers,primary,no check,%s\n", address)
			hosts, err := ParseCsvHostsWithServerAddresses(csv, "writers")
			if err == nil || len(hosts) != 0 {
				t.Fatalf("hosts = %+v, err = %v; want an error without a partial roster", hosts, err)
			}
		})
	}
	for _, csv := range []string{
		"# pxname,svname,status\nwriters,primary,no check\n",
		"# pxname,svname,status,addr\nwriters,primary,no check\n",
		"# pxname,svname,status,addr\nwriters,primary\n",
		"# pxname,svname,addr\nwriters,primary,192.0.2.10:3306\n",
	} {
		if hosts, err := ParseCsvHostsWithServerAddresses(csv, "writers"); err == nil || len(hosts) != 0 {
			t.Errorf("CSV %q: hosts = %+v, err = %v; want a schema error", csv, hosts, err)
		}
	}
}

func TestServerAddressStatusAndTransitionFiltering(t *testing.T) {
	csv := "# pxname,svname,status,addr\n" +
		"writers,primary,no check,192.0.2.10:3307\n" +
		"writers,down,DOWN,192.0.2.11:3308\n" +
		"writers,up,UP,192.0.2.12:3309\n" +
		"writers,disabled,NOLB,192.0.2.13:3310\n" +
		"writers,transitioning,UP 1/2,192.0.2.14:3311\n"
	hosts, err := ParseCsvHostsWithServerAddresses(csv, "writers")
	if err != nil {
		t.Fatal(err)
	}
	got := FilterThrottlerBackendHosts(hosts)
	want := []*BackendHost{
		{Hostname: "192.0.2.10", Port: 3307, Status: StatusNoCheck},
		{Hostname: "192.0.2.11", Port: 3308, Status: StatusDown},
		{Hostname: "192.0.2.12", Port: 3309, Status: StatusUp},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered hosts = %+v, want %+v", got, want)
	}
	for _, status := range []string{"UP 1/2", "DOWN 1/2"} {
		csv := fmt.Sprintf("# pxname,svname,status,addr\nwriters,primary,%s,192.0.2.10:3307\n", status)
		if _, err := ParseCsvHostsWithServerAddresses(csv, "writers"); err == nil {
			t.Errorf("all hosts transitioning (%s): expected an error", status)
		}
	}
}
