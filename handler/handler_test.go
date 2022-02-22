package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/castai/azure-spot-handler/castai"
)

func TestRunLoop(t *testing.T) {
	r := require.New(t)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	nodeName := "AI"
	castNodeID := "CAST"

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				CastNodeIDLabel: castNodeID,
			},
		},
		Spec: v1.NodeSpec{
			Unschedulable: false,
		},
	}

	t.Run("handle successful mock interruption", func(t *testing.T) {
		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++
			var req castai.CloudEventRequest
			r.NoError(json.NewDecoder(re.Body).Decode(&req))
			r.Equal(req.NodeID, castNodeID)
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(node)
		castHttp := castai.NewDefaultClient(castS.URL, "test", log.Level, 100*time.Millisecond, "0.0.0")
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval: 100 * time.Millisecond,
			interruptChecker: mockInterrupt,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)

		node, _ = fakeApi.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.Equal(true, node.Spec.Unschedulable)
	})

	t.Run("handle mock interruption retries", func(t *testing.T) {
		m := sync.Mutex{}

		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			// locking for predictable test result
			m.Lock()
			mothershipCalls++

			if mothershipCalls >= 3 {
				var req castai.CloudEventRequest
				r.NoError(json.NewDecoder(re.Body).Decode(&req))
				r.Equal(req.NodeID, castNodeID)
				w.WriteHeader(http.StatusOK)
				m.Unlock()
			} else {
				m.Unlock()
				log.Infof("Mothership hanging")
				time.Sleep(time.Millisecond * 100)
				log.Infof("Mothership responding")
				w.WriteHeader(http.StatusGatewayTimeout)
			}
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(node)
		castHttp := castai.NewDefaultClient(castS.URL, "test", log.Level, time.Millisecond*100, "0.0.0")
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval: time.Millisecond * 100,
			interruptChecker: mockInterrupt,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := handler.Run(ctx)
		require.NoError(t, err)

		defer func() {
			cancel()
			r.Equal(3, mothershipCalls)
		}()
	})
}

type mockInterruptChecker struct {
	interrupted bool
}

func (m *mockInterruptChecker) Check(ctx context.Context) (bool, error) {
	return m.interrupted, nil
}
