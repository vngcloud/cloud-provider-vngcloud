package cni_detector

import (
	"context"

	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// CNIType represents the CNI type detected in the cluster.
type CNIType string

const (
	CalicoOverlay       CNIType = "Calico Overlay"
	CiliumOverlay       CNIType = "Cilium Overlay"
	CiliumNativeRouting CNIType = "Cilium Native Routing"
	UnknownCNI          CNIType = "Unknown CNI"
)

// Detector detects the CNI type used in the Kubernetes cluster.
type Detector struct {
	kubeClient kubernetes.Interface
}

// NewDetector creates a new instance of the CNI Detector.
func NewDetector(kubeClient kubernetes.Interface) *Detector {
	return &Detector{kubeClient: kubeClient}
}

// DetectCNIType detects the CNI type in the cluster.
func (d *Detector) DetectCNIType() (CNIType, error) {
	// Check for Calico
	if d.isCalicoOverlay() {
		return CalicoOverlay, nil
	}

	// Check for Cilium
	if d.isCiliumNativeRouting() {
		return CiliumNativeRouting, nil
	}

	if d.isCiliumOverlay() {
		return CiliumOverlay, nil
	}

	return UnknownCNI, nil
}

// Check if Calico Overlay is running
func (d *Detector) isCalicoOverlay() bool {
	calicoNodeDaemonSet, err := d.kubeClient.AppsV1().DaemonSets("kube-system").Get(context.TODO(), "calico-node", apimetav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error getting Calico DaemonSet: %v", err)
		return false
	}
	if calicoNodeDaemonSet == nil {
		return false
	}
	return true
}

// Check if Cilium Overlay is running
func (d *Detector) isCiliumOverlay() bool {
	ciliumDaemonSet, err := d.kubeClient.AppsV1().DaemonSets("kube-system").Get(context.TODO(), "cilium", apimetav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error getting Cilium DaemonSet: %v", err)
		return false
	}
	if ciliumDaemonSet == nil {
		return false
	}
	return true
}

// Check if Cilium Native Routing is running
func (d *Detector) isCiliumNativeRouting() bool {
	if d.isCiliumOverlay() {
		// get cilium-config config map
		ciliumConfigMap, err := d.kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "cilium-config", apimetav1.GetOptions{})
		if err != nil {
			klog.Errorf("Error getting Cilium ConfigMap: %v", err)
			return false
		}
		if ciliumConfigMap == nil {
			return false
		}
		// check if cilium-config have routing-mode: native
		if ciliumConfigMap.Data["routing-mode"] == "native" {
			return true
		}
	}
	return false
}
