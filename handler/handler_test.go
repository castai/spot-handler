package handler

import (
	"context"
	"github.com/sirupsen/logrus"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTaintNode(t *testing.T) {
	r := require.New(t)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	t.Run("handle node interruption by tainting successfully", func (t *testing.T) {
		nodeName := "AI"
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
			Spec: v1.NodeSpec{
				Unschedulable: false,
			},
		}

		clientset := fake.NewSimpleClientset(node)

		handler := AzureSpotHandler{
			log: log,
			clientset: clientset,
			nodeName: nodeName,
		}

		err := handler.handleInterruption(context.Background())
		r.NoError(err)

		node, err = clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.Equal(true, node.Spec.Unschedulable)
	})
}
