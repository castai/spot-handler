package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/compute/metadata"
	"github.com/stretchr/testify/require"
)

func TestGCPInterruptChecker(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/computeMetadata/v1/instance/maintenance-event", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		_, err := res.Write([]byte("NO_MAINTENANCE"))
		require.NoError(t, err)
	})
	mux.HandleFunc("/computeMetadata/v1/instance/preempted", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		_, err := res.Write([]byte("TRUE"))
		require.NoError(t, err)
	})

	s := httptest.NewServer(mux)
	defer s.Close()

	os.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(s.URL, "http://"))
	checker := gcpInterruptChecker{
		metadata: metadata.NewClient(nil),
	}

	interrupted, err := checker.Check(context.Background())
	require.NoError(t, err)
	require.True(t, interrupted)
}
