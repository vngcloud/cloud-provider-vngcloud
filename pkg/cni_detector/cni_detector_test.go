package cni_detector

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
)

func TestDetectCNIType_CalicoOverlay(t *testing.T) {
	// Set up a fake Kubernetes client with Calico DaemonSet
	client := fake.NewSimpleClientset()
	client.AppsV1().DaemonSets("kube-system").Create(context.TODO(), &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "calico-node",
			Namespace: "kube-system",
		},
	}, metav1.CreateOptions{})

	detector := &Detector{kubeClient: client}
	cniType, err := detector.DetectCNIType()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cniType != CalicoOverlay {
		t.Errorf("expected CNI type %s, got %s", CalicoOverlay, cniType)
	}
}

func TestDetectCNIType_CiliumOverlay(t *testing.T) {
	// Set up a fake Kubernetes client with Cilium DaemonSet
	client := fake.NewSimpleClientset()
	client.AppsV1().DaemonSets("kube-system").Create(context.TODO(), &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cilium",
			Namespace: "kube-system",
		},
	}, metav1.CreateOptions{})

	detector := &Detector{kubeClient: client}
	cniType, err := detector.DetectCNIType()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cniType != CiliumOverlay {
		t.Errorf("expected CNI type %s, got %s", CiliumOverlay, cniType)
	}
}

func TestDetectCNIType_CiliumNativeRouting(t *testing.T) {
	// Set up a fake Kubernetes client with Cilium DaemonSet and config map
	client := fake.NewSimpleClientset()

	// Create the Cilium DaemonSet
	client.AppsV1().DaemonSets("kube-system").Create(context.TODO(), &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cilium",
			Namespace: "kube-system",
		},
	}, metav1.CreateOptions{})

	// Create the Cilium ConfigMap with routing-mode: native
	client.CoreV1().ConfigMaps("kube-system").Create(context.TODO(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cilium-config",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"routing-mode": "native",
		},
	}, metav1.CreateOptions{})

	detector := &Detector{kubeClient: client}
	cniType, err := detector.DetectCNIType()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cniType != CiliumNativeRouting {
		t.Errorf("expected CNI type %s, got %s", CiliumNativeRouting, cniType)
	}
}

func TestDetectCNIType_UnknownCNI(t *testing.T) {
	// Set up a fake Kubernetes client without any CNI configuration
	client := fake.NewSimpleClientset()

	detector := &Detector{kubeClient: client}
	cniType, err := detector.DetectCNIType()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cniType != UnknownCNI {
		t.Errorf("expected CNI type %s, got %s", UnknownCNI, cniType)
	}
}

func TestRealCluster(t *testing.T) {
	// configPath := "/home/annd2/Downloads/annd2-clean.txt"
	configPath := "/home/annd2/Downloads/cluster-43dd8bab-fd8.txt"
	if configPath == "" {
		t.Skip("Skipping test; no kubeconfig provided")
	}
	// init new kubernetes client

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		t.Fatalf("failed to create Kubernetes client: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to create Kubernetes client: %v", err)
	}

	detector := NewDetector(clientset)
	cniType, err := detector.DetectCNIType()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	t.Logf("Detected CNI type: %s", cniType)
}
