package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGCPInterruptChecker(t *testing.T) {
	checker := gcpInterruptChecker{
		metadata: mockMetadata{},
	}

	interrupted, err := checker.Check(context.Background())
	require.NoError(t, err)
	require.True(t, interrupted)
}

type mockMetadata struct {
}

func (md mockMetadata) Get(path string) (string, error) {
	m := map[string]string{
		"instance/maintenance-event": "NO_MAINTENANCE",
		"instance/preempted":         "TRUE",
	}
	return m[path], nil
}
