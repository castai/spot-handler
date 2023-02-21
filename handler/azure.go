package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// NewAzureInterruptChecker checks for azure spot interrupt event from metadata server.
// See https://docs.microsoft.com/en-us/azure/virtual-machines/linux/scheduled-events#endpoint-discovery
func NewAzureInterruptChecker() MetadataChecker {
	client := resty.New()
	// Times out if set to 1 second, after 2 we will try again soon anyway
	client.SetTimeout(time.Second * 2)

	return &azureInterruptChecker{
		client:            client,
		metadataServerURL: "http://169.254.169.254",
	}
}

type azureInterruptChecker struct {
	client            *resty.Client
	metadataServerURL string
}

type azureSpotScheduledEvent struct {
	EventType string
}
type azureSpotScheduledEvents struct {
	Events []azureSpotScheduledEvent
}

func (c *azureInterruptChecker) CheckInterrupt(ctx context.Context) (bool, error) {
	responseBody := azureSpotScheduledEvents{}

	req := c.client.NewRequest().SetContext(ctx).SetResult(&responseBody)
	req.SetHeader("Metadata", "true")
	resp, err := req.Get(fmt.Sprintf("%s/metadata/scheduledevents?api-version=2020-07-01", c.metadataServerURL))
	if err != nil {
		return false, fmt.Errorf("getting metadata/preemtied: %w", err)
	}

	if resp.StatusCode() != 200 {
		return false, fmt.Errorf("received unexpected status code: %d", resp.StatusCode())
	}

	for _, e := range responseBody.Events {
		if e.EventType == "Preempt" {
			return true, nil
		}
	}

	return false, nil
}

func (c *azureInterruptChecker) CheckRebalanceRecommendation(ctx context.Context) (bool, error) {
	// Applicable only for AWS for now.
	return false, nil
}
