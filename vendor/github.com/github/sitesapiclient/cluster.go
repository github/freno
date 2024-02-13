package sitesapiclient

// Cluster represents a Kubernetes cluster that exists in our
// datacenters that is composed of many compute instances
type Cluster struct {
	ClusterGroup      string   `json:"cluster_group"`
	InstanceAttribute string   `json:"instance_attribute"`
	Instances         []string `json:"instances"`
	Name              string   `json:"name"`
	Site              string   `json:"site"`
	Status	          string   `json:"status"`
}

// ListClusters will retrieve all clusters or a list of clusters based on query
// parameteres that are passed
func (c *Client) ListClusters(params map[string]string) ([]Cluster, error) {
	req, err := c.NewRequest("GET", "/clusters", params, nil)
	if err != nil {
		return nil, err
	}
	var clusters []Cluster
	_, err = c.do(req, &clusters)
	return clusters, err
}

// FindCluster will retrieve a single cluster by name
func (c *Client) FindCluster(name string) (*Cluster, error) {
	req, err := c.NewRequest("GET", "/clusters/"+name, nil, nil)
	if err != nil {
		return nil, err
	}
	var cluster Cluster
	_, err = c.do(req, &cluster)
	return &cluster, err
}
