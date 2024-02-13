package glb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	sitesapi "github.com/github/sitesapiclient"
)

// Status Gets the GLB status output for a running service
type Status struct {
	Datacenter  string
	Service     string
	Coordiators []string
	Endpoint    string
	Scheme      string
}

var (
	// ErrNoCoordinatorNodes is returned when we cannot find any glb coordinators through sitesAPI
	ErrNoCoordinatorNodes = errors.New("no coordinator nodes found")
)

// NewStatus initializes a status object with the default https scheme
func NewStatus(service, datacenter string, c *sitesapi.Client, scheme string) (*Status, error) {
	return NewStatusWithScheme(service, datacenter, c, "https")
}

// NewStatusWithScheme initializes a Status object and sets the connection http scheme
func NewStatusWithScheme(service, datacenter string, c *sitesapi.Client, scheme string) (*Status, error) {
	if c == nil {
		return nil, errors.New("sitesapi client cannot be nil")
	}

	nodelist, err := GetCoordinatorNodes(c, datacenter)
	if err != nil {
		return nil, err
	}

	if len(nodelist) <= 0 {
		return nil, ErrNoCoordinatorNodes
	}

	s := Status{
		Datacenter:  datacenter,
		Service:     service,
		Coordiators: nodelist,
		Scheme:      scheme,
	}
	ep, err := s.PoolEndpoints()
	if err != nil {
		return nil, err
	}
	s.Endpoint = ep[0]

	return &s, nil
}

// Refresh returns a new status output
func (s *Status) Refresh() (*StatusMessage, error) {
	if len(s.Endpoint) == 0 {
		return nil, errors.New("no stats endpoint defined")
	}

	URL := fmt.Sprintf("%s/;csv;norefresh", s.Endpoint)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Get(URL)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	msg, err := NewStatusMessage(resp.Body)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// InternalLoadBalancersResponse is the structure of the json response from
//  /api/v1/clusters/{cluster}/load_balancers/{pool}/stats
type InternalLoadBalancersResponse struct {
	Cluster           string
	Service           string
	HAProxyStatsPorts map[string][]string `json:"haproxy_stats_ports"`
}

// PoolEndpoints find the haproxy servers we can query for the actual status
func (s *Status) PoolEndpoints() ([]string, error) {
	result := []string{}

	// pick any coordinator
	server := s.Coordiators[0]
	URL := fmt.Sprintf("%s://%s/api/v1/clusters/internal/load_balancers/%s/stats",
		s.Scheme, server, s.Service)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Get(URL)
	if err != nil {
		return result, err
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}

	message := InternalLoadBalancersResponse{}
	err = json.Unmarshal(data, &message)
	if err != nil {
		return result, err
	}

	// Just return one. The whole list is overkill.
	for _, val := range message.HAProxyStatsPorts {
		if len(val) > 0 {
			result = append(result, val[0])
			return result, nil
		}
	}
	return result, errors.New("no stats endpoints found")
}

// GetCoordinatorNodes searches sitesapi for the correct set of glb nodes
func GetCoordinatorNodes(c *sitesapi.Client, datacenter string) ([]string, error) {
	results := []string{}
	instances, err := c.ListInstances(map[string]string{
		"enabled":    "true",
		"deployable": "true",
		"app":        "glb",
		"role":       "coordinator",
		"site":       datacenter,
	})

	if err != nil {
		return results, err
	}

	for _, i := range instances {
		results = append(results, i.Hostname)
	}

	return results, nil
}
