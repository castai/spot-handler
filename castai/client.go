package castai

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

const (
	headerAPIKey    = "X-API-Key"
	headerUserAgent = "User-Agent"
)

type Client interface {
	SendCloudEvent(ctx context.Context, req *CloudEventRequest) error
}

func NewClient(log *logrus.Logger, rest *resty.Client, clusterID string) Client {
	return &client{
		log:       log,
		rest:      rest,
		clusterID: clusterID,
	}
}

// NewRestyClient configures a default instance of the resty.Client used to do HTTP requests.
func NewRestyClient(url, key, ca string, level logrus.Level, timeout time.Duration, version string) (*resty.Client, error) {
	clientTransport, err := createHTTPTransport(ca)
	if err != nil {
		return nil, err
	}
	client := resty.NewWithClient(&http.Client{
		Transport: clientTransport,
	})
	client.SetBaseURL(url)
	client.SetTimeout(timeout)
	client.Header.Set(headerAPIKey, key)
	client.Header.Set(headerUserAgent, "castai-spot-handler/"+version)
	if level == logrus.TraceLevel {
		client.SetDebug(true)
	}

	return client, nil
}

func createHTTPTransport(ca string) (*http.Transport, error) {
	tlsConfig, err := createTLSConfig(ca)
	if err != nil {
		return nil, fmt.Errorf("creating TLS config: %v", err)
	}
	// Mostly copied from http.DefaultTransport.
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}, nil
}

func createTLSConfig(ca string) (*tls.Config, error) {
	if len(ca) == 0 {
		return nil, nil
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM([]byte(ca)) {
		return nil, fmt.Errorf("failed to add root certificate to CA pool")
	}

	return &tls.Config{
		RootCAs: certPool,
	}, nil
}

type client struct {
	log       *logrus.Logger
	rest      *resty.Client
	clusterID string
}

type CloudEventRequest struct {
	EventType string `json:"event_type"`
	NodeID    string `json:"node_id"`
}

func (c *client) SendCloudEvent(ctx context.Context, req *CloudEventRequest) error {
	resp, err := c.rest.R().
		SetBody(req).
		SetContext(ctx).
		Post(fmt.Sprintf("/v1/kubernetes/external-clusters/%s/events", c.clusterID))

	if err != nil {
		return fmt.Errorf("sending aks spot interrupt: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("sending aks spot interrupt: request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}

	return nil
}
