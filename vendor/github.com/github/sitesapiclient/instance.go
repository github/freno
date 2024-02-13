package sitesapiclient

import "fmt"

// Instance represents a single compute resource
type Instance struct {
	Attributes         map[string]string `json:"attributes"`
	Hostname           string            `json:"hostname"`
	IPAddressPrivate   string            `json:"private_ipv4_address"`
	IPAddressPublic    string            `json:"public_ipv4_address"`
	Provider           string            `json:"provider"`
	ProviderInstanceID string            `json:"provider_instance_id"`
	Site               string            `json:"site"`
}

// InstanceNotFoundError is returned  when an instance could not be found, to
// differentiate that from a server error.
type InstanceNotFoundError struct {
	InstanceHostname string // the instance hostname that couldn't be found
}

func (e *InstanceNotFoundError) Error() string {
	return fmt.Sprintf("instance not found: %s", e.InstanceHostname)
}

// ListInstances will retrieve a full list of instances or a set of parameters
// can be provided to query by.
func (c *Client) ListInstances(params map[string]string) ([]Instance, error) {
	req, err := c.NewRequest("GET", "/instances", params, nil)
	if err != nil {
		return nil, err
	}
	var instances []Instance
	_, err = c.do(req, &instances)
	return instances, err
}

// FindInstance will retrieve a single instance by hostname
func (c *Client) FindInstance(name string) (*Instance, error) {
	req, err := c.NewRequest("GET", "/instances/"+name, nil, nil)
	if err != nil {
		return nil, err
	}
	var instance Instance
	_, err = c.do(req, &instance)
	// return any server-side failures immediately, since a server error
	// would otherwise match the below check and return InstanceNotFoundError.
	if err != nil {
		if statusError, isStatusError := err.(StatusError); isStatusError && statusError.Code == 404 {
			return nil, &InstanceNotFoundError{InstanceHostname: name}
		} else {
			return nil, err
		}
	}

	if instance.ProviderInstanceID == "" {
		return nil, &InstanceNotFoundError{InstanceHostname: name}
	}

	return &instance, err
}

// SetAttribute will write a single attribute to an instance
func (c *Client) SetAttribute(name string, key string, value string) error {
	body := map[string]map[string]string{"attributes": {key: value}}
	req, err := c.NewRequest("PUT", "/instances/"+name, nil, body)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}
