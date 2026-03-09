package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

// NewAzureInterruptChecker checks for azure spot interrupt event from metadata server.
// See https://docs.microsoft.com/en-us/azure/virtual-machines/linux/scheduled-events#endpoint-discovery
func NewAzureInterruptChecker(log logrus.FieldLogger) MetadataChecker {
	client := resty.New()
	// Times out if set to 1 second, after 2 we will try again soon anyway
	client.SetTimeout(time.Second * 2)

	return &azureInterruptChecker{
		client:            client,
		metadataServerURL: "http://169.254.169.254",
		log:               log,
	}
}

type azureInterruptChecker struct {
	client            *resty.Client
	metadataServerURL string
	log               logrus.FieldLogger
}

// azureSpotScheduledEvent is a single event schema, not all fields are necessarily mapped
// see https://learn.microsoft.com/en-us/azure/virtual-machines/linux/scheduled-events#the-basics for details
type azureSpotScheduledEvent struct {
	EventId     string
	EventType   string
	EventStatus string
	EventSource string
	Description string
	NotBefore   string
}
type azureSpotScheduledEvents struct {
	DocumentIncarnation int
	Events              []azureSpotScheduledEvent
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

	if len(responseBody.Events) == 0 {
		return false, nil
	}

	c.log.Debugf("Received %d scheduled events with incarnation %d", len(responseBody.Events), responseBody.DocumentIncarnation)

	for _, e := range responseBody.Events {
		c.log.Debugf("Scheduled event seen: EventId=%s, EventType=%s, EventStatus=%s, EventSource=%s, NotBefore=%s, Description=%s",
			e.EventId, e.EventType, e.EventStatus, e.EventSource, e.NotBefore, e.Description)
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
