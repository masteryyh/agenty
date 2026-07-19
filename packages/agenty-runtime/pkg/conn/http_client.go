package conn

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	sharedHTTPClient     *http.Client
	sharedHTTPClientOnce sync.Once
)

func GetHTTPClient() *http.Client {
	sharedHTTPClientOnce.Do(func() {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			TLSHandshakeTimeout:   10 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 0,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
		}
		sharedHTTPClient = &http.Client{
			Transport: transport,
		}
	})
	return sharedHTTPClient
}
