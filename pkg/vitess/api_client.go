package vitess

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/github/freno/pkg/base"
	"vitess.io/vitess/go/vt/proto/topodata"
)

const defaultTimeout = time.Duration(5.0) * time.Second

// Tablet represents information about a running instance of vttablet.
type Tablet struct {
	MysqlHostname string              `json:"mysql_hostname,omitempty"`
	MysqlPort     int32               `json:"mysql_port,omitempty"`
	Type          topodata.TabletType `json:"type,omitempty"`
}

// IsValidReplica returns a bool reflecting if a tablet type is REPLICA
func (t Tablet) IsValidReplica() bool {
	return t.Type == topodata.TabletType_REPLICA
}

func constructAPIURL(api string, keyspace string, shard string) (url string) {
	api = strings.TrimRight(api, "/")
	if !strings.HasSuffix(api, "/api") {
		api = fmt.Sprintf("%s/api", api)
	}
	url = fmt.Sprintf("%s/keyspace/%s/tablets/%s", api, keyspace, shard)

	return url
}

// filterReplicaTablets parses a list of tablets, returning replica tablets only
func filterReplicaTablets(tablets []Tablet) (replicas []Tablet) {
	for _, tablet := range tablets {
		if tablet.IsValidReplica() {
			replicas = append(replicas, tablet)
		}
	}
	return replicas
}

// Client gathers info from the Vitess API
type Client struct {
	client *http.Client
}

// New returns a new Client
func New() *Client {
	return &Client{client: base.SetupHttpClient(0)}
}

// ParseTablets reads from vitess /api/ks_tablets/<keyspace>/[shard] and returns a
// listing (mysql_hostname, mysql_port, type) of REPLICA tablets
func (c *Client) ParseTablets(api, keyspace, shard string, timeoutSecs float64) (tablets []Tablet, err error) {
	if timeoutSecs == 0 {
		c.client.Timeout = defaultTimeout
	} else {
		c.client.Timeout = time.Duration(timeoutSecs) * time.Second
	}

	url := constructAPIURL(api, keyspace, shard)
	resp, err := c.client.Get(url)
	if err != nil {
		return tablets, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return tablets, err
	}

	err = json.Unmarshal(body, &tablets)
	return filterReplicaTablets(tablets), err
}
