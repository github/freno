package vitess

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// Tablet represents information about a running instance of vttablet.
type Tablet struct {
	MysqlHostname string `json:"mysql_hostname,omitempty"`
	MysqlPort     int32  `json:"mysql_port,omitempty"`
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
// tblet (mysql_hostname, mysql_port) listing
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
