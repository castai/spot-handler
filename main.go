package main

import (
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"azure-spot-handler/handler"
	"azure-spot-handler/internal/config"
	"azure-spot-handler/internal/version"
)

var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

func main() {
	cfg := config.Get()

	logger := logrus.New()
	log := logrus.WithFields(logrus.Fields{})
	httpClient := NewDefaultClient()

	kubeconfig, err := retrieveKubeConfig(log)
	if err != nil {
		log.Fatalf("err retrieving kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("err creating clientset: %w", err)
	}

	k8sVersion, err := version.Get(clientset)
	if err != nil {
		log.Fatalf("failed getting kubernetes version: %v", err)
	}

	handlerVersion := &version.HandlerVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	log = logger.WithFields(logrus.Fields{
		"version":     handlerVersion,
		"k8s_version": k8sVersion.Full(),
	})

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Fatalf("node name not provided")
	}

	spotHandler := handler.NewHandler(log, httpClient, clientset, cfg.PollIntervalSeconds, nodeName)

	if cfg.PprofPort != 0 {
		go func() {
			addr := fmt.Sprintf(":%d", cfg.PprofPort)
			log.Infof("starting pprof server on %s", addr)
			if err := http.ListenAndServe(addr, http.DefaultServeMux); err != nil {
				log.Errorf("failed to start pprof http server: %v", err)
			}
		}()
	}

	if err := spotHandler.Run(signals.SetupSignalHandler()); err != nil {
		logErr := &logContextErr{}
		if errors.As(err, &logErr) {
			log = logger.WithFields(logErr.fields)
		}
		log.Fatalf("azure-spot-handler failed: %v", err)
	}
}

// NewDefaultClient configures a default instance of the resty.Client used to do HTTP requests.
func NewDefaultClient() *resty.Client {
	client := resty.New()

	// times out if set to 1 second, after 2 we will try again soon anyway
	client.SetTimeout(time.Second * 2)

	return client
}

func retrieveKubeConfig(log logrus.FieldLogger) (*rest.Config, error) {
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	log.Debug("using in cluster kubeconfig")
	return inClusterConfig, nil
}

type logContextErr struct {
	err    error
	fields logrus.Fields
}
