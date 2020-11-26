package vitess

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/github/freno/pkg/config"
	"vitess.io/vitess/go/vt/proto/topodata"
)

const defaultTimeout = time.Duration(5) * time.Second

// TabletRealtimeStats represents realtime stats from a running instance of vttablet.
type TabletRealtimeStats struct {
	HealthError string `json:"health_error,omitempty"`
}

// TabletStats represents stats from a running instance of vttablet.
type TabletStats struct {
	LastError string               `json:"last_error,omitempty"`
	Realtime  *TabletRealtimeStats `json:"realtime,omitempty"`
	Serving   bool                 `json:"serving,omitempty"`
	Up        bool                 `json:"up,omitempty"`
}

// Tablet represents information about a running instance of vttablet.
type Tablet struct {
	Alias         *topodata.TabletAlias `json:"alias,omitempty"`
	MysqlHostname string                `json:"mysql_hostname,omitempty"`
	MysqlPort     int32                 `json:"mysql_port,omitempty"`
	Stats         *TabletStats          `json:"stats,omitempty"`
	Type          topodata.TabletType   `json:"type,omitempty"`
}

// HasValidCell returns a bool reflecting if a tablet is in a valid Vitess cell
func (t Tablet) HasValidCell(validCells []string) bool {
	if len(validCells) == 0 {
		return true
	}
	for _, cell := range validCells {
		if t.Alias.GetCell() == cell {
			return true
		}
	}
	return false
}

// IsServeable returns a bool reflecting if a tablet is eligible to serve traffic based on tablet stats. For
// backwards-compatibilty tablets are assumed to be healthy if realtime stats is disabled. This method aims
// to mimic the logic used by vtgate to select tablets for read queries without considering minimum tablet
// count (not important to freno) and replication lag (freno polls its own replication lag)
func (t Tablet) IsServeable() bool {
	if t.Stats != nil {
		if t.Stats.Realtime != nil && t.Stats.Realtime.HealthError != "" {
			return false
		}
		return t.Stats.Serving && t.Stats.Up && t.Stats.LastError == ""
	}
	return true
}

// IsValidReplica returns a bool reflecting if a tablet type is REPLICA
func (t Tablet) IsValidReplica() bool {
	if t.Type != topodata.TabletType_REPLICA {
		return false
	}
	return t.IsServeable()
}

var httpClient = http.Client{
	Timeout: defaultTimeout,
}

func constructAPIURL(settings config.VitessConfigurationSettings) (url string) {
	api := strings.TrimRight(settings.API, "/")
	if !strings.HasSuffix(api, "/api") {
		api = fmt.Sprintf("%s/api", api)
	}
	url = fmt.Sprintf("%s/keyspace/%s/tablets/%s", api, settings.Keyspace, settings.Shard)

	return url
}

// ParseCells returns a slice of non-empty Vitess cell names
func ParseCells(settings config.VitessConfigurationSettings) (cells []string) {
	for _, cell := range settings.Cells {
		cell = strings.TrimSpace(cell)
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}

// filterReplicaTablets parses a list of tablets, returning replica tablets only
func filterReplicaTablets(settings config.VitessConfigurationSettings, tablets []Tablet) (replicas []Tablet) {
	validCells := ParseCells(settings)
	for _, tablet := range tablets {
		if tablet.HasValidCell(validCells) && tablet.IsValidReplica() {
			replicas = append(replicas, tablet)
		}
	}
	return replicas
}

// ParseTablets reads from vitess /api/keyspace/<keyspace>/tablets/[shard] and returns a
// listing (mysql_hostname, mysql_port, type) of REPLICA tablets
func ParseTablets(settings config.VitessConfigurationSettings) (tablets []Tablet, err error) {
	if settings.TimeoutSecs == 0 {
		httpClient.Timeout = defaultTimeout
	} else {
		httpClient.Timeout = time.Duration(settings.TimeoutSecs) * time.Second
	}

	url := constructAPIURL(settings)
	resp, err := httpClient.Get(url)
	if err != nil {
		return tablets, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tablets, fmt.Errorf("%v", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return tablets, err
	}
	err = json.Unmarshal(body, &tablets)
	return filterReplicaTablets(settings, tablets), err
}
