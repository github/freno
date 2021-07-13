package mysql

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"time"

	"github.com/go-sql-driver/mysql"
)

// NewURL creates a new string representing the URI for the mysql driver to connect to, including timeout, charset and tls settings.
// In case the URL cannot be created due to a wrong TLS configuration, an error is returned.
func NewURL(hostname string, port int, databaseName, user, password, tlsCaCerPath, tlsClientCertPath, tlsClientKeyPath string, tlsSkipVerify bool, timeout time.Duration) (*url.URL, error) {
	var err error
	tlsKey := "false"

	if tlsCaCerPath != "" || tlsClientCertPath != "" || tlsClientKeyPath != "" {
		tlsKey = fmt.Sprintf("%s:%d", hostname, port)
		err = registerTlsConfig(tlsKey, tlsCaCerPath, tlsClientCertPath, tlsClientKeyPath, tlsSkipVerify)
		if err != nil {
			return nil, err
		}
	}

	ip := net.ParseIP(hostname)
	if (ip != nil) && (ip.To4() == nil) {
		// Wrap IPv6 literals in square brackets
		hostname = fmt.Sprintf("[%s]", hostname)
	}

	s := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&charset=utf8mb4,utf8,latin1&tls=%s&timeout=%dms", user, password, hostname, port, databaseName, tlsKey, timeout.Milliseconds())
	return url.Parse(s)
}

// registerTlsConfig registers the certificates under a given key which is calculated based on the
// paths of the certificates, and returns that key, or an error if the certificates couldn't be registered.
func registerTlsConfig(key, tlsCaCertificatePath, tlsClientCertPath, tlsClientKeyPath string, tlsSkipVerify bool) (err error) {
	var cert tls.Certificate
	var rootCertPool *x509.CertPool
	var pem []byte

	if tlsCaCertificatePath == "" {
		rootCertPool, err = x509.SystemCertPool()
		if err != nil {
			return
		}
	} else {
		rootCertPool = x509.NewCertPool()
		pem, err = ioutil.ReadFile(tlsCaCertificatePath)
		if err != nil {
			return
		}
		if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
			err = errors.New("cannot add ca certificate to cert pool")
			return
		}
	}
	if tlsClientCertPath != "" || tlsClientKeyPath != "" {
		cert, err = tls.LoadX509KeyPair(tlsClientCertPath, tlsClientKeyPath)
		if err != nil {
			return
		}
	}

	config := tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            rootCertPool,
		InsecureSkipVerify: tlsSkipVerify,
	}

	err = mysql.RegisterTLSConfig(key, &config)
	return
}
