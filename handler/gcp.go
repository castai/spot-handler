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

// NewGCPChecker checks for gcp spot interrupt event from metadata server.
func NewGCPChecker() InterruptChecker {
	return &gcpInterruptChecker{
		metadata: metadata.NewClient(nil),
	}
}

type gcpInterruptChecker struct {
	metadata *metadata.Client
}

func (c *gcpInterruptChecker) Check(ctx context.Context) (bool, error) {
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
