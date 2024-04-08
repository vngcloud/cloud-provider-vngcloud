package vngcloud

import (
	"context"
	"reflect"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
)

type servicePatcher struct {
	kubeClient kubernetes.Interface
	base       *corev1.Service
	updated    *corev1.Service
}

func newServicePatcher(pKubeClient kubernetes.Interface, pBase *corev1.Service) servicePatcher {
	return servicePatcher{
		kubeClient: pKubeClient,
		base:       pBase.DeepCopy(),
		updated:    pBase,
	}
}

// Patch will submit a patch request for the Service unless the updated service
// reference contains the same set of annotations as the base copied during
// servicePatcher initialization.
func (sp *servicePatcher) Patch(ctx context.Context, err error) error {
	if reflect.DeepEqual(sp.base.Annotations, sp.updated.Annotations) {
		return err
	}
	perr := utils.PatchService(ctx, sp.kubeClient, sp.base, sp.updated)
	return errors.NewAggregate([]error{err, perr})
}
