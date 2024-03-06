//go:generate mockgen -destination ./mock/version.go . Interface
package version

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	Full() string
	MinorInt() int
}

func Get(log *logrus.Entry, clientset kubernetes.Interface) (Interface, error) {
	cs, ok := clientset.(*kubernetes.Clientset)
	if !ok {
		return nil, fmt.Errorf("expected clientset to be of type *kubernetes.Clientset but was %T", clientset)
	}

	var sv *version.Info
	err := backoff.Retry(func() error {
		var err error
		sv, err = cs.ServerVersion()
		if err != nil {
			log.Warnf("failed getting server version, retrying: %v", err)
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 5))
	if err != nil {
		return nil, fmt.Errorf("getting server version: %w", err)
	}

	m, err := strconv.Atoi(regexp.MustCompile(`^(\d+)`).FindString(sv.Minor))
	if err != nil {
		return nil, fmt.Errorf("parsing minor version: %w", err)
	}

	return &Version{v: sv, m: m}, nil
}

type Version struct {
	v *version.Info
	m int
}

func (v *Version) Full() string {
	return v.v.Major + "." + v.v.Minor
}

func (v *Version) MinorInt() int {
	return v.m
}

type HandlerVersion struct {
	GitCommit, GitRef, Version string
}

func (a *HandlerVersion) String() string {
	return fmt.Sprintf("GitCommit=%q GitRef=%q Version=%q", a.GitCommit, a.GitRef, a.Version)
}
