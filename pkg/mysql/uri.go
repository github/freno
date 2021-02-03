package mysql

import (
	"fmt"
	"net"
	"time"
)

const timeout = 10 * time.Millisecond

// MakeUri creates a new string representing the URI for the mysql driver to connect to, including timeout, charset and tls settings.
// In case the URI cannot be created due to a wrong TLS configuration, an error is returned.
func MakeUri(hostname string, port int, databaseName, user, password string, timeout time.Duration) (uri string, err error) {
	tlsKey := "false"

	ip := net.ParseIP(hostname)
	if (ip != nil) && (ip.To4() == nil) {
		// Wrap IPv6 literals in square brackets
		hostname = fmt.Sprintf("[%s]", hostname)
	}

	uri = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&charset=utf8mb4,utf8,latin1&tls=%s&timeout=%dms", user, password, hostname, port, databaseName, tlsKey, timeout.Milliseconds())
	return uri, err
}
