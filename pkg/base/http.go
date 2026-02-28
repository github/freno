package base

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

var defaultTimeout = time.Second

func SetupHttpClient(httpTimeout time.Duration) *http.Client {
	if httpTimeout == 0 {
		httpTimeout = time.Second
	}
	dialTimeout := func(network, addr string) (net.Conn, error) {
		return net.DialTimeout(network, addr, httpTimeout)
	}
	httpTransport := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
		Dial:                  dialTimeout,
		ResponseHeaderTimeout: httpTimeout,
	}
	httpClient := &http.Client{Transport: httpTransport}

	return httpClient
}
