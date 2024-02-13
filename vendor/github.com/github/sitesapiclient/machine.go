package sitesapiclient

// Machine represents a single machine resource
type Machine struct {
	Attributes        map[string]string `json:"attributes"`
	Provider          string            `json:"provider"`
	ProviderMachineID string            `json:"provider_machine_id"`
	Site              string            `json:"site"`
}

// ListMachines will retrieve a full list of machines or a set of parameters
// can be provided to query by.
func (c *Client) ListMachines(params map[string]string) ([]Machine, error) {
	req, err := c.NewRequest("GET", "/machines", params, nil)
	if err != nil {
		return nil, err
	}
	var machines []Machine
	_, err = c.do(req, &machines)
	return machines, err
}
