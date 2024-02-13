package glb

import (
	"fmt"

	"github.com/github/go-db/internal/gitlib"
	yaml "gopkg.in/yaml.v2"
)

// http://glb-proxy-0123bfa.ash1-iad.github.net:2801/;csv;norefresh

// Service represents a glb-service and its config
type Service struct {
	Name   string
	Suffix string
	Config ServiceConfig
}

// ServiceConfig is the representation of the yaml file from the service
type ServiceConfig struct {
	Nbproc int
	Sites  map[string]Site
}

// Site holds a map of bindings
type Site struct {
	Binds map[string][]string
}

// NewService loads the GLB service config from github
func NewService(name, suffix string, helper gitlib.Helper) (*Service, error) {
	service := Service{
		Name:   name,
		Suffix: suffix,
	}

	// get a reference to the master branch
	branch, err := helper.GetExistingBranch("glb", "master")
	if err != nil {
		return nil, err
	}

	// load the config file
	configFile := fmt.Sprintf("services/%s/%s.yml", name, name)
	data, err := branch.GetFileBlob(configFile)
	if err != nil {
		return nil, err
	}

	jerr := yaml.Unmarshal(data, &service.Config)
	if jerr != nil {
		return &service, jerr
	}
	return &service, nil
}
