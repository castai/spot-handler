package handler

import (
	"context"
	"encoding/json"
	"github.com/castai/azure-spot-handler/internal/castai"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRunLoop(t *testing.T) {
	r := require.New(t)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	nodeName := "AI"
	castNodeName := "CAST"

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string {
				CastNodeIDLabel: castNodeName,
			},
		},
		Spec: v1.NodeSpec{
			Unschedulable: false,
		},
	}

	t.Run("handle successful mock interruption", func (t *testing.T) {
		mothershipCalls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++

			var req castai.CloudEventRequest
			json.NewDecoder(re.Body).Decode(&req)
			r.Equal(req.NodeID, castNodeName)
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(node)
		castHttp := castai.NewDefaultClient(castS.URL, "test", log.Level, 1)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockHttp := NewDefaultClient(s.URL)

		handler := AzureSpotHandler{
			pollWaitInterval: 1,
			client: mockHttp,
			castClient: mockCastClient,
			nodeName: nodeName,
			clientset: fakeApi,
			log: log,
		}


		ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
		handler.Run(ctx)

		defer func(){
			cancel()
			r.Equal(1, mothershipCalls)
			node, _ = fakeApi.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
			r.Equal(true, node.Spec.Unschedulable)
		}()
	})

	t.Run("handle mock interruption retries", func (t *testing.T) {
		mothershipCalls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++

			if mothershipCalls >= 3 {
				var req castai.CloudEventRequest
				json.NewDecoder(re.Body).Decode(&req)
				r.Equal(req.NodeID, castNodeName)
				w.WriteHeader(http.StatusOK)
			} else {
				log.Infof("Mothership hanging")
				time.Sleep(time.Second * 5)
				log.Infof("Mothership responding")
				w.WriteHeader(http.StatusGatewayTimeout)
			}
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(node)
		castHttp := castai.NewDefaultClient(castS.URL, "test", log.Level, 1)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockHttp := NewDefaultClient(s.URL)

		handler := AzureSpotHandler{
			pollWaitInterval: 1,
			client: mockHttp,
			castClient: mockCastClient,
			nodeName: nodeName,
			clientset: fakeApi,
			log: log,
		}


		ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
		handler.Run(ctx)

		defer func(){
			cancel()
			r.Equal(3, mothershipCalls)
		}()
	})
}
