package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"
)

func TestGCPInterruptChecker(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/computeMetadata/v1/instance/preempted", r.URL.String())

		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("TRUE"))
		require.NoError(t, err)
	}))
	defer s.Close()

	checker := gcpInterruptChecker{
		client:            resty.New(),
		metadataServerURL: s.URL,
	}

	interrupted, err := checker.Check(context.Background())
	require.NoError(t, err)
	require.True(t, interrupted)
}
