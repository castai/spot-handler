package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"
)

func TestAzureInterruptChecker(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/metadata/scheduledevents?api-version=2020-07-01", r.URL.String())

		mockInterrupt := azureSpotScheduledEvent{
			EventType: "Preempt",
		}
		eventsWrapper := azureSpotScheduledEvents{
			Events: []azureSpotScheduledEvent{mockInterrupt},
		}

		b, err := json.Marshal(eventsWrapper)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(b)
		require.NoError(t, err)
	}))
	defer s.Close()

	checker := azureInterruptChecker{
		client:            resty.New(),
		metadataServerURL: s.URL,
	}

	interrupted, err := checker.CheckInterrupt(context.Background())
	require.NoError(t, err)
	require.True(t, interrupted)
}
