package sitesapiclient

import "fmt"

// Site represents a datacenter, physical or virtual, that exists in
// our inventory.
type Site struct {
	Aliases   []string `json:"aliases"`
	CIDRs     []string `json:"cidrs"`
	GpanelURL string   `json:"gpanel_url"`
	ID        string   `json:"id"`
	Provider  string   `json:"provider"`
	Region    string   `json:"region"`
}

// Sites holds information
type Sites []Site

// Filter will take a map of sites and determine if the site needed exists
func (s Sites) Filter(name string) (Site, error) {
	for _, site := range s {
		if site.ID == name {
			return site, nil
		}
	}
	return Site{}, fmt.Errorf("Site %s does not exist", name)
}

// ListSites will retrieve a list of all registered sites and can be filtered
// by a set of parameters
func (c *Client) ListSites() (Sites, error) {
	req, err := c.NewRequest("GET", "/sites", map[string]string{}, nil)
	if err != nil {
		return nil, err
	}
	var sites Sites
	_, err = c.do(req, &sites)
	return sites, err
}
