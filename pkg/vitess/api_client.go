package vitess

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"vitess.io/vitess/go/vt/proto/topodata"
)

// Tablet represents information about a running instance of vttablet.
type Tablet struct {
	MysqlHostname string              `json:"mysql_hostname,omitempty"`
	MysqlPort     int32               `json:"mysql_port,omitempty"`
	Type          topodata.TabletType `json:"type,omitempty"`
}

var httpClient = http.Client{
	Timeout: 1 * time.Second,
}

func constructAPIURL(api string, keyspace string, shard string) (url string) {
	api = strings.TrimRight(api, "/")
	if !strings.HasSuffix(api, "/api") {
		api = fmt.Sprintf("%s/api", api)
	}
	url = fmt.Sprintf("%s/keyspace/%s/tablets/%s", api, keyspace, shard)

	return url
}

// ParseTablets reads from vitess /api/ks_tablets/<keyspace>/[shard] and returns a
// tablet (mysql_hostname, mysql_port) listing
func ParseTablets(api string, keyspace string, shard string) (tablets []Tablet, err error) {
	url := constructAPIURL(api, keyspace, shard)
	resp, err := httpClient.Get(url)
	if err != nil {
		return tablets, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return tablets, err
	}

	err = json.Unmarshal(body, &tablets)
	return tablets, err
}

func isReplicaTablet(tablet Tablet) bool {
	return tablet.Type == topodata.TabletType_REPLICA
}

// FilterReplicaTablets parses a list of tablets, returning replica tablets only
func FilterReplicaTablets(tablets []Tablet) []Tablet {
	replicas := make([]Tablet, 0)
	for _, tablet := range tablets {
		if isReplicaTablet(tablet) {
			replicas = append(replicas, tablet)
		}
	}
	return replicas
}
