package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"

	"github.com/castai/spot-handler/castai"
)

const CastNodeIDLabel = "provisioner.cast.ai/node-id"

const (
	taintNodeDraining       = "autoscaling.cast.ai/draining"
	taintNodeDrainingEffect = "NoSchedule"

	labelNodeDraining                  = "autoscaling.cast.ai/draining"
	valueNodeDrainingReasonInterrupted = "spot-interruption"

	cloudEventInterrupted             = "interrupted"
	cloudEventRebalanceRecommendation = "rebalanceRecommendation"

	valueTrue = "true"
)

type MetadataChecker interface {
	CheckInterrupt(ctx context.Context) (bool, error)
	CheckRebalanceRecommendation(ctx context.Context) (bool, error)
}

type SpotHandler struct {
	castClient       castai.Client
	clientset        kubernetes.Interface
	metadataChecker  MetadataChecker
	nodeName         string
	pollWaitInterval time.Duration
	log              logrus.FieldLogger
	gracePeriod      time.Duration
}

func NewSpotHandler(
	log logrus.FieldLogger,
	castClient castai.Client,
	clientset kubernetes.Interface,
	metadataChecker MetadataChecker,
	pollWaitInterval time.Duration,
	nodeName string,
) *SpotHandler {
	return &SpotHandler{
		castClient:       castClient,
		clientset:        clientset,
		metadataChecker:  metadataChecker,
		log:              log,
		nodeName:         nodeName,
		pollWaitInterval: pollWaitInterval,
		gracePeriod:      30 * time.Second,
	}
}

func (g *SpotHandler) Run(ctx context.Context) error {
	t := time.NewTicker(g.pollWaitInterval)
	defer t.Stop()

	var once sync.Once
	deadline := time.NewTimer(24 * 365 * time.Hour)

	// Once rebalance recommendation is set by cloud it stays there permanently. It needs to be sent only once.
	var rebalanceRecommendationSent bool

	for {
		select {
		case <-t.C:
			err := func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				interrupted, err := g.metadataChecker.CheckInterrupt(ctx)
				if err != nil {
					return err
				}
				if interrupted {
					g.log.Infof("preemption notice received")
					if err := g.handleInterruption(ctx); err != nil {
						return err
					}
					// Stop after ACK.
					t.Stop()
				}

				if !rebalanceRecommendationSent {
					rebalanceRecommendation, err := g.metadataChecker.CheckRebalanceRecommendation(ctx)
					if err != nil {
						return err
					}
					if rebalanceRecommendation {
						g.log.Infof("rebalance recommendation notice received")
						if err := g.handleRebalanceRecommendation(ctx); err != nil {
							return err
						}
						rebalanceRecommendationSent = true
					}
				}

				return nil
			}()

			if err != nil {
				g.log.Errorf("checking for cloud events: %v", err)
			}
		case <-deadline.C:
			return nil
		case <-ctx.Done():
			// Signal received, starting countdown until exiting the loop.
			once.Do(func() {
				deadline.Reset(g.gracePeriod)
			})
		}
	}
}

func (g *SpotHandler) handleInterruption(ctx context.Context) error {
	node, err := g.clientset.CoreV1().Nodes().Get(ctx, g.nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	req := &castai.CloudEventRequest{
		EventType: cloudEventInterrupted,
		NodeID:    node.Labels[CastNodeIDLabel],
	}
	if node.Spec.ProviderID != "" {
		req.ProviderID = &node.Spec.ProviderID
	}
	if err = g.castClient.SendCloudEvent(ctx, req); err != nil {
		return err
	}

	return g.taintNode(ctx, node)
}

func (g *SpotHandler) taintNode(ctx context.Context, node *v1.Node) error {
	if node.Spec.Unschedulable {
		return nil
	}

	err := g.patchNode(ctx, node, func(n *v1.Node) error {
		n.Spec.Unschedulable = true
		n.Labels[labelNodeDraining] = valueNodeDrainingReasonInterrupted
		n.Spec.Taints = append(n.Spec.Taints, v1.Taint{
			Key:    taintNodeDraining,
			Value:  valueTrue,
			Effect: taintNodeDrainingEffect,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("patching node unschedulable: %w", err)
	}
	return nil
}

func (g *SpotHandler) patchNode(ctx context.Context, node *v1.Node, changeFn func(*v1.Node) error) error {
	oldData, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshaling old data: %w", err)
	}

	if err := changeFn(node); err != nil {
		return err
	}

	newData, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshaling new data: %w", err)
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, node)
	if err != nil {
		return fmt.Errorf("creating patch for node: %w", err)
	}

	err = backoff.Retry(func() error {
		_, err = g.clientset.CoreV1().Nodes().Patch(ctx, node.Name, apitypes.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return err
	}, defaultBackoff(ctx))
	if err != nil {
		return fmt.Errorf("patching node: %w", err)
	}

	return nil
}

func defaultBackoff(ctx context.Context) backoff.BackOffContext {
	return backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 5), ctx)
}

func (g *SpotHandler) handleRebalanceRecommendation(ctx context.Context) error {
	node, err := g.clientset.CoreV1().Nodes().Get(ctx, g.nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	req := &castai.CloudEventRequest{
		EventType: cloudEventRebalanceRecommendation,
		NodeID:    node.Labels[CastNodeIDLabel],
	}
	if node.Spec.ProviderID != "" {
		req.ProviderID = &node.Spec.ProviderID
	}

	return g.castClient.SendCloudEvent(ctx, req)
}
