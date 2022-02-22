package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// NewGCPChecker checks for gcp spot interrupt event from metadata server.
func NewGCPChecker() InterruptChecker {
	client := resty.New()
	// Times out if set to 1 second, after 2 we will try again soon anyway
	client.SetTimeout(time.Second * 2)

	return &gcpInterruptChecker{
		client:            client,
		metadataServerURL: "http://metadata.google.internal",
	}
}

type gcpInterruptChecker struct {
	client *resty.Client

	metadataServerURL string
}

func (c *gcpInterruptChecker) Check(ctx context.Context) (bool, error) {
	req := c.client.NewRequest().SetContext(ctx)
	req.SetHeader("Metadata-Flavor", "Google")
	resp, err := req.Get(fmt.Sprintf("%s/computeMetadata/v1/instance/preempted", c.metadataServerURL))
	if err != nil {
		return false, fmt.Errorf("getting metadata/preemtied: %w", err)
	}

	if resp.StatusCode() != 200 {
		return false, fmt.Errorf("received unexpected status code: %d", resp.StatusCode())
	}

	if string(resp.Body()) == "FALSE" {
		return false, nil
	}

	return true, nil
}
