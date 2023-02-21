package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-node-termination-handler/pkg/ec2metadata"
	"github.com/stretchr/testify/require"
)

func TestAwsInterruptChecker(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("/latest/api/token", func(writer http.ResponseWriter, request *http.Request) {

		writer.Header().Set("X-aws-ec2-metadata-token-ttl-seconds", "1000")
		fmt.Fprintf(writer, "TOKEN")
	})
	router.HandleFunc("/latest/meta-data/spot/instance-action", func(writer http.ResponseWriter, request *http.Request) {
		action := ec2metadata.InstanceAction{
			Action: "SPOT_INT",
			Time:   time.Now().Format(time.RFC3339),
		}
		b, err := json.Marshal(action)
		require.NoError(t, err)

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, err = writer.Write(b)
		require.NoError(t, err)
	})
	s := httptest.NewServer(router)
	defer s.Close()

	checker := awsInterruptChecker{
		imds: ec2metadata.New(s.URL, 3),
	}

	interrupted, err := checker.CheckInterrupt(context.Background())
	require.NoError(t, err)
	require.True(t, interrupted)
}
