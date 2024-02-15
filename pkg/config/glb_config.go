package config

// GLB configuration specific to github's internal setup

import (
	"github.com/github/go-db/glb"
	"github.com/github/sitesapiclient"

	"github.com/outbrain/golib/log"
)

var glbSettings = newGLBSettings()

type GLBSettings struct {
	ProxyReadOnly string
	ProxyWriters  string
	ProxyTesting  string
}

func newGLBSettings() *GLBSettings {
	return &GLBSettings{}
}

// GLB returns global GLBSettings struct
func GLB() *GLBSettings {
	return glbSettings
}

// Load loads the GLB MySQL pool endpoints.
func (glb *GLBSettings) Load(sitesClient *sitesapiclient.Client, site string) error {
	log.Infof("site: %s", site)
	roProxy, err := poolEndpoint(sitesClient, "mysql-proxy", site)
	if err != nil {
		return err
	}
	rwProxy, err := poolEndpoint(sitesClient, "mysql-proxy-writers", site)
	if err != nil {
		return err
	}
	testingProxy, err := poolEndpoint(sitesClient, "mysql-proxy-testing", site)
	if err != nil {
		return err
	}

	glb.ProxyReadOnly = roProxy
	glb.ProxyWriters = rwProxy
	glb.ProxyTesting = testingProxy
	return nil
}

func poolEndpoint(sitesClient *sitesapiclient.Client, pool, site string) (string, error) {
	status, err := glb.NewStatus(pool, site, sitesClient, "https")
	if err != nil {
		return "", err
	}
	endpoint, err := status.PoolEndpoints()
	if err != nil {
		return "", nil
	}
	return endpoint[0], nil
}
