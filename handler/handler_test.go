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

	"github.com/castai/spot-handler/castai"
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
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval:  100 * time.Millisecond,
			metadataChecker:   mockInterrupt,
			castClient:        mockCastClient,
			nodeName:          nodeName,
			clientset:         fakeApi,
			log:               log,
			phase2Permissions: true,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)

		node, _ = fakeApi.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.Equal(true, node.Spec.Unschedulable)
		r.Equal(valueNodeDrainingReasonInterrupted, node.Labels[labelNodeDraining])
		r.Contains(node.Spec.Taints, v1.Taint{
			Key:    taintNodeDraining,
			Value:  valueTrue,
			Effect: taintNodeDrainingEffect,
		})
	})

	t.Run("do not taint node if not enough permissions", func(t *testing.T) {
		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++
			var req castai.CloudEventRequest
			r.NoError(json.NewDecoder(re.Body).Decode(&req))
			r.Equal(req.NodeID, castNodeID)
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		node2 := &v1.Node{
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
		fakeApi := fake.NewSimpleClientset(node2)
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test2")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval:  100 * time.Millisecond,
			metadataChecker:   mockInterrupt,
			castClient:        mockCastClient,
			nodeName:          nodeName,
			clientset:         fakeApi,
			log:               log,
			phase2Permissions: false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)

		node2, _ = fakeApi.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.Equal(false, node2.Spec.Unschedulable)
		r.NotEqual(valueNodeDrainingReasonInterrupted, node2.Labels[labelNodeDraining])
		r.NotContains(node2.Spec.Taints, v1.Taint{
			Key:    taintNodeDraining,
			Value:  valueTrue,
			Effect: taintNodeDrainingEffect,
		})
	})

	t.Run("keep checking interruption on context canceled", func(t *testing.T) {
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
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval: 1 * time.Second,
			metadataChecker:  mockInterrupt,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
			gracePeriod:      2 * time.Second,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)

		node, _ = fakeApi.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.Equal(true, node.Spec.Unschedulable)
		r.Equal(valueNodeDrainingReasonInterrupted, node.Labels[labelNodeDraining])
		r.Contains(node.Spec.Taints, v1.Taint{
			Key:    taintNodeDraining,
			Value:  valueTrue,
			Effect: taintNodeDrainingEffect,
		})
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
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, time.Millisecond*100, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval: time.Millisecond * 100,
			metadataChecker:  mockInterrupt,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = handler.Run(ctx)
		require.NoError(t, err)

		defer func() {
			cancel()
			r.Equal(3, mothershipCalls)
		}()
	})

	t.Run("handle successful mock rebalance recommendation", func(t *testing.T) {
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
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockRecommendation := &mockInterruptChecker{rebalanceRecommendation: true}
		handler := SpotHandler{
			pollWaitInterval: 100 * time.Millisecond,
			metadataChecker:  mockRecommendation,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)
	})

	t.Run("populate providerID in interruption event", func(t *testing.T) {
		providerID := "aws:///us-east-1a/i-1234567890abcdef0"
		nodeWithProviderID := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					CastNodeIDLabel: castNodeID,
				},
			},
			Spec: v1.NodeSpec{
				Unschedulable: false,
				ProviderID:    providerID,
			},
		}

		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++
			var req castai.CloudEventRequest
			r.NoError(json.NewDecoder(re.Body).Decode(&req))
			r.Equal(castNodeID, req.NodeID)
			r.NotNil(req.ProviderID, "ProviderID should not be nil")
			r.Equal(providerID, *req.ProviderID, "ProviderID should match node's Spec.ProviderID")
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(nodeWithProviderID)
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval: 100 * time.Millisecond,
			metadataChecker:  mockInterrupt,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)
	})

	t.Run("populate providerID in rebalance recommendation event", func(t *testing.T) {
		providerID := "gce://my-project/us-central1-a/instance-123"
		nodeWithProviderID := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					CastNodeIDLabel: castNodeID,
				},
			},
			Spec: v1.NodeSpec{
				Unschedulable: false,
				ProviderID:    providerID,
			},
		}

		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++
			var req castai.CloudEventRequest
			r.NoError(json.NewDecoder(re.Body).Decode(&req))
			r.Equal(castNodeID, req.NodeID)
			r.NotNil(req.ProviderID, "ProviderID should not be nil")
			r.Equal(providerID, *req.ProviderID, "ProviderID should match node's Spec.ProviderID")
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(nodeWithProviderID)
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockRecommendation := &mockInterruptChecker{rebalanceRecommendation: true}
		handler := SpotHandler{
			pollWaitInterval: 100 * time.Millisecond,
			metadataChecker:  mockRecommendation,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)
	})

	t.Run("override providerID in interruption event", func(t *testing.T) {
		originalProviderID := "aws:///us-east-1a/i-1234567890abcdef0"
		overrideProviderID := "aws:///us-east-1b/i-0987654321fedcba0"
		nodeWithOverride := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					CastNodeIDLabel: castNodeID,
				},
				Annotations: map[string]string{
					OverrideProviderIDAnnot: overrideProviderID,
				},
			},
			Spec: v1.NodeSpec{
				Unschedulable: false,
				ProviderID:    originalProviderID,
			},
		}

		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++
			var req castai.CloudEventRequest
			r.NoError(json.NewDecoder(re.Body).Decode(&req))
			r.Equal(castNodeID, req.NodeID)
			r.NotNil(req.ProviderID, "ProviderID should not be nil")
			r.Equal(overrideProviderID, *req.ProviderID, "ProviderID should use override annotation value")
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(nodeWithOverride)
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockInterrupt := &mockInterruptChecker{interrupted: true}
		handler := SpotHandler{
			pollWaitInterval: 100 * time.Millisecond,
			metadataChecker:  mockInterrupt,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)
	})

	t.Run("override providerID in rebalance recommendation event", func(t *testing.T) {
		originalProviderID := "gce://my-project/us-central1-a/instance-123"
		overrideProviderID := "gce://my-project/us-central1-b/instance-456"
		nodeWithOverride := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					CastNodeIDLabel: castNodeID,
				},
				Annotations: map[string]string{
					OverrideProviderIDAnnot: overrideProviderID,
				},
			},
			Spec: v1.NodeSpec{
				Unschedulable: false,
				ProviderID:    originalProviderID,
			},
		}

		mothershipCalls := 0
		castS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, re *http.Request) {
			mothershipCalls++
			var req castai.CloudEventRequest
			r.NoError(json.NewDecoder(re.Body).Decode(&req))
			r.Equal(castNodeID, req.NodeID)
			r.NotNil(req.ProviderID, "ProviderID should not be nil")
			r.Equal(overrideProviderID, *req.ProviderID, "ProviderID should use override annotation value")
			w.WriteHeader(http.StatusOK)
		}))
		defer castS.Close()

		fakeApi := fake.NewSimpleClientset(nodeWithOverride)
		castHttp, err := castai.NewRestyClient(castS.URL, "test", "", log.Level, 100*time.Millisecond, "0.0.0")
		r.NoError(err)
		mockCastClient := castai.NewClient(log, castHttp, "test1")

		mockRecommendation := &mockInterruptChecker{rebalanceRecommendation: true}
		handler := SpotHandler{
			pollWaitInterval: 100 * time.Millisecond,
			metadataChecker:  mockRecommendation,
			castClient:       mockCastClient,
			nodeName:         nodeName,
			clientset:        fakeApi,
			log:              log,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err = handler.Run(ctx)
		require.NoError(t, err)
		r.Equal(1, mothershipCalls)
	})
}

type mockInterruptChecker struct {
	interrupted             bool
	rebalanceRecommendation bool
}

func (m *mockInterruptChecker) CheckInterrupt(ctx context.Context) (bool, error) {
	return m.interrupted, nil
}

func (m *mockInterruptChecker) CheckRebalanceRecommendation(ctx context.Context) (bool, error) {
	return m.rebalanceRecommendation, nil
}
