package util

import (
	"fmt"
	"net/url"
	"regexp"
)

var (
	EnvVariableRegexp = regexp.MustCompile("[$][{](.*?)[}]")
)

type HostPort struct {
	Host string
	Port int
}

func (h *HostPort) String() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

func (h *HostPort) URL() *url.URL {
	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", h.Host, h.Port))
	return u
}
