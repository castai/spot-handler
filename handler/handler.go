package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
)

type AzureSpotHandler struct {
	client    *resty.Client
	clientset *kubernetes.Clientset
	nodeName  string
	log       logrus.FieldLogger
}

type azureSpotScheduledEvent struct {
	EventType string
}
type azureSpotScheduledEvents struct {
	Events []azureSpotScheduledEvent
}

// https://docs.microsoft.com/en-us/azure/virtual-machines/linux/scheduled-events#endpoint-discovery
const azureScheduledEventsBackend = "http://169.254.169.254/metadata/scheduledevents?api-version=2020-07-01"

func NewHandler(
	log logrus.FieldLogger,
	client *resty.Client,
	clientset *kubernetes.Clientset,
	nodeName string) *AzureSpotHandler {
	return &AzureSpotHandler{
		client:    client,
		clientset: clientset,
		log:       log,
		nodeName:  nodeName,
	}
}

func (g *AzureSpotHandler) Run(ctx context.Context) error {
	g.log.Infof("Azure Spot handler starting for node: %s", g.nodeName)
	t := time.NewTicker(time.Second * 3)
	defer t.Stop()

	for {
		select {
		case <-t.C:
		case <-ctx.Done():
			// check interruption
			err := func() error {
				interrupted, err := g.CheckInterruption(ctx)
				if err != nil {
					return err
				}
				if interrupted {
					g.log.Infof("preemption notice received")
					return g.HandleInterruption(ctx)
				}

				g.log.Infof("Not interrupted")

				return nil
			}()

			if err != nil {
				g.log.Errorf("checking for interruption: %v", err)
			}
		}
	}
}

func (g *AzureSpotHandler) CheckInterruption(ctx context.Context) (bool, error) {
	responseBody := azureSpotScheduledEvents{}

	req := g.client.NewRequest().SetContext(ctx).SetResult(&responseBody)
	req.SetHeader("Metadata", "true")
	resp, err := req.Get(azureScheduledEventsBackend)
	if err != nil {
		return false, fmt.Errorf("getting metadata/preemtied: %w", err)
	}

	if resp.StatusCode() != 200 {
		return false, fmt.Errorf("received unexpected status code: %d", resp.StatusCode())
	}

	for _, e := range responseBody.Events {
		if e.EventType == "Preempt" {
			return true, nil
		}
	}

	return false, nil
}

func (g *AzureSpotHandler) GetSelfNode(ctx context.Context) (*v1.Node, error) {
	g.log.Debugf("getting node: %s", g.nodeName)
	node, err := g.clientset.CoreV1().Nodes().Get(ctx, g.nodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			g.log.Info("node not found")
			return nil, err
		}
		return nil, err
	}
	return node, nil
}

func (g *AzureSpotHandler) HandleInterruption(ctx context.Context) error {
	selfNode, err := g.GetSelfNode(ctx)
	if err != nil {
		return err
	}

	return g.taintNode(ctx, selfNode)
}

func (g *AzureSpotHandler) taintNode(ctx context.Context, node *v1.Node) error {
	if node.Spec.Unschedulable {
		return nil
	}

	err := g.patchNode(ctx, node, func(n *v1.Node) error {
		n.Spec.Unschedulable = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("patching node unschedulable: %w", err)
	}
	return nil
}

func (g *AzureSpotHandler) patchNode(ctx context.Context, node *v1.Node, changeFn func(*v1.Node) error) error {
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
