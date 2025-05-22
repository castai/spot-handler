package main

import (
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/castai/spot-handler/castai"
	"github.com/castai/spot-handler/config"
	"github.com/castai/spot-handler/handler"
	"github.com/castai/spot-handler/version"
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

	kubeconfig, err := retrieveKubeConfig(log, cfg)
	if err != nil {
		log.Fatalf("err retrieving kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("err creating clientset: %v", err)
	}

	k8sVersion, err := version.Get(log, clientset)
	if err != nil {
		log.Warnf("failed getting kubernetes version: %v", err)
	}

	handlerVersion := &version.HandlerVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	log = logger.WithFields(logrus.Fields{
		"version": handlerVersion,
	})

	k8sVersionField := "unknown"
	if k8sVersion != nil {
		k8sVersionField = k8sVersion.Full()
	}

	log = log.WithFields(logrus.Fields{
		"k8s_version": k8sVersionField,
	})

	interruptChecker, err := buildInterruptChecker(cfg.Provider)
	if err != nil {
		log.Fatalf("interrupt checker: %v", err)
	}

	// Set 5 seconds until we timeout calling mothership and retry.
	castHttpClient, err := castai.NewRestyClient(
		cfg.APIUrl,
		cfg.APIKey,
		cfg.TLSCACert,
		logrus.Level(cfg.LogLevel),
		5*time.Second,
		Version,
	)
	if err != nil {
		log.Fatalf("failed to create http client: %v", err)
	}
	castClient := castai.NewClient(logger, castHttpClient, cfg.ClusterID)

	spotHandler := handler.NewSpotHandler(
		log,
		castClient,
		clientset,
		interruptChecker,
		time.Duration(cfg.PollIntervalSeconds)*time.Second,
		cfg.NodeName,
	)

	if cfg.PprofPort != 0 {
		go func() {
			addr := fmt.Sprintf(":%d", cfg.PprofPort)
			log.Infof("starting pprof server on %s", addr)
			if err := http.ListenAndServe(addr, http.DefaultServeMux); err != nil {
				log.Errorf("failed to start pprof http server: %v", err)
			}
		}()
	}

	log.Infof("running spot handler, provider=%s", cfg.Provider)
	if err := spotHandler.Run(signals.SetupSignalHandler()); err != nil {
		logErr := &logContextErr{}
		if errors.As(err, &logErr) {
			log = logger.WithFields(logErr.fields)
		}
		log.Fatalf("spot handler failed: %v", err)
	}
}

func buildInterruptChecker(provider string) (handler.MetadataChecker, error) {
	switch provider {
	case "azure":
		return handler.NewAzureInterruptChecker(), nil
	case "gcp":
		return handler.NewGCPChecker(), nil
	case "aws":
		return handler.NewAWSInterruptChecker(), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func kubeConfigFromEnv(cfg config.Config) (*rest.Config, error) {
	kubepath := cfg.Kubeconfig
	if kubepath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(kubepath)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig at %s: %w", kubepath, err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("building rest config from kubeconfig at %s: %w", kubepath, err)
	}

	return restConfig, nil
}

func retrieveKubeConfig(log logrus.FieldLogger, cfg config.Config) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromEnv(cfg)
	if err != nil {
		return nil, err
	}

	if kubeconfig != nil {
		log.Debug("using kubeconfig from env variables")
		return kubeconfig, nil
	}

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

func (e *logContextErr) Error() string {
	return e.err.Error()
}

func (e *logContextErr) Unwrap() error {
	return e.err
}
