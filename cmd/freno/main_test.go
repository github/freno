package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/github/freno/pkg/config"
	"github.com/outbrain/golib/log"
)

func TestLogMySQLSafetyConfiguration(t *testing.T) {
	originalLogLevel := log.GetLevel()
	log.SetLevel(log.ERROR)
	defer log.SetLevel(originalLogLevel)

	recoveryThreshold := 3.0
	settings := config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"legacy": {ThrottleThreshold: 5},
			"dependency": {
				RequiredClusters:  []string{"primary"},
				ThrottleThreshold: 5,
				User:              "do-not-log-user",
				Password:          "do-not-log-password",
				MetricQuery:       "do-not-log-query",
			},
			"primary": {ThrottleThreshold: 50},
			"shadow-only": {
				ShadowRequiredClusters: []string{"primary"},
				ThrottleThreshold:      5,
			},
			"fail-closed": {
				FailOnNoHosts:     true,
				ThrottleThreshold: 50,
			},
			"recovery": {
				RecoveryDurationMillis: 5000,
				RecoveryThreshold:      &recoveryThreshold,
				ThrottleThreshold:      10,
			},
		},
	}
	var output bytes.Buffer
	if err := logMySQLSafetyConfiguration(settings, &output); err != nil {
		t.Fatal(err)
	}
	want := "freno MySQL safety configuration: cluster=\"dependency\" RequiredClusters=[\"primary\"] ShadowRequiredClusters=[] RecoveryDurationMillis=0 RecoveryThreshold=5 ThrottleThreshold=5 FailOnNoHosts=false\n" +
		"freno MySQL safety configuration: cluster=\"fail-closed\" RequiredClusters=[] ShadowRequiredClusters=[] RecoveryDurationMillis=0 RecoveryThreshold=50 ThrottleThreshold=50 FailOnNoHosts=true\n" +
		"freno MySQL safety configuration: cluster=\"recovery\" RequiredClusters=[] ShadowRequiredClusters=[] RecoveryDurationMillis=5000 RecoveryThreshold=3 ThrottleThreshold=10 FailOnNoHosts=false\n" +
		"freno MySQL safety configuration: cluster=\"shadow-only\" RequiredClusters=[] ShadowRequiredClusters=[\"primary\"] RecoveryDurationMillis=0 RecoveryThreshold=5 ThrottleThreshold=5 FailOnNoHosts=false\n"
	if got := output.String(); got != want {
		t.Fatalf("startup safety configuration = %q, want %q", got, want)
	}
	if log.GetLevel() != log.ERROR {
		t.Fatal("startup safety notices changed unrelated log filtering")
	}
}

func TestLogMySQLSafetyConfigurationWithoutSafetySettings(t *testing.T) {
	settings := config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"legacy": {ThrottleThreshold: 5},
		},
	}
	var output bytes.Buffer
	if err := logMySQLSafetyConfiguration(settings, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("unexpected safety notice for legacy configuration: %q", output.String())
	}
}

func TestLogMySQLSafetyConfigurationEscapesNames(t *testing.T) {
	settings := config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"line\nbreak": {RequiredClusters: []string{"probe\nname"}},
		},
	}
	var output bytes.Buffer
	if err := logMySQLSafetyConfiguration(settings, &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, `cluster="line\nbreak" RequiredClusters=["probe\nname"]`) {
		t.Fatalf("cluster names did not stay within one escaped log line: %q", got)
	}
}

type failingLogWriter struct {
	err error
}

func (writer failingLogWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func TestLogMySQLSafetyConfigurationWriteError(t *testing.T) {
	settings := config.MySQLConfigurationSettings{
		Clusters: map[string]*config.MySQLClusterConfigurationSettings{
			"primary": {FailOnNoHosts: true},
		},
	}
	writeErr := errors.New("log destination unavailable")
	err := logMySQLSafetyConfiguration(settings, failingLogWriter{err: writeErr})
	if !errors.Is(err, writeErr) || !strings.Contains(err.Error(), `"primary"`) {
		t.Fatalf("expected contextual startup logging error, got %v", err)
	}
}
