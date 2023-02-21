package handler

import (
	"context"

	"cloud.google.com/go/compute/metadata"
)

const (
	maintenanceEventTerminate = "TERMINATE_ON_HOST_MAINTENANCE"
	preemptionEventTrue       = "TRUE"

	maintenanceSuffix = "instance/maintenance-event"
	preemptionSuffix  = "instance/preempted"
)

type metadataGetter interface {
	Get(path string) (string, error)
}

// NewGCPChecker checks for gcp spot interrupt event from metadata server.
func NewGCPChecker() MetadataChecker {
	return &gcpInterruptChecker{
		metadata: metadata.NewClient(nil),
	}
}

type gcpInterruptChecker struct {
	metadata metadataGetter
}

func (c *gcpInterruptChecker) CheckInterrupt(ctx context.Context) (bool, error) {
	m, err := c.metadata.Get(maintenanceSuffix)
	if err != nil {
		return false, err
	}
	p, err := c.metadata.Get(preemptionSuffix)
	if err != nil {
		return false, err
	}

	return m == maintenanceEventTerminate || p == preemptionEventTrue, nil
}

func (c *gcpInterruptChecker) CheckRebalanceRecommendation(ctx context.Context) (bool, error) {
	// Applicable only for AWS for now.
	return false, nil
}
