package utils

import (
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/errors"
	apiv1 "k8s.io/api/core/v1"
	nwv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	nwlisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// create k8s client from config file
func CreateApiserverClient(apiserverHost string, kubeConfig string) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(apiserverHost, kubeConfig)
	if err != nil {
		return nil, err
	}

	cfg.QPS = consts.DefaultQPS
	cfg.Burst = consts.DefaultBurst
	cfg.ContentType = "application/vnd.kubernetes.protobuf"

	log.Debug("creating kubernetes API client")

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	v, err := client.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"version": fmt.Sprintf("v%v.%v", v.Major, v.Minor),
	}).Debug("kubernetes API client created")

	return client, nil
}

func GetIngress(ingressLister nwlisters.IngressLister, key string) (*nwv1.Ingress, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, err
	}

	ingress, err := ingressLister.Ingresses(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return ingress, nil
}

func GetService(serviceLister corelisters.ServiceLister, key string) (*apiv1.Service, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, err
	}

	service, err := serviceLister.Services(namespace).Get(name)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func GetServiceNodePort(serviceLister corelisters.ServiceLister, name string, serviceBackend *nwv1.IngressServiceBackend) (int, error) {
	var portInfo intstr.IntOrString
	if serviceBackend.Port.Name != "" {
		portInfo.Type = intstr.String
		portInfo.StrVal = serviceBackend.Port.Name
	} else {
		portInfo.Type = intstr.Int
		portInfo.IntVal = serviceBackend.Port.Number
	}

	svc, err := GetService(serviceLister, name)
	if err != nil {
		return 0, err
	}

	var nodePort int
	ports := svc.Spec.Ports
	for _, p := range ports {
		if portInfo.Type == intstr.Int && int(p.Port) == portInfo.IntValue() {
			nodePort = int(p.NodePort)
			break
		}
		if portInfo.Type == intstr.String && p.Name == portInfo.StrVal {
			nodePort = int(p.NodePort)
			break
		}
	}

	if nodePort == 0 {
		return 0, fmt.Errorf("failed to find nodeport for service %s:%s", name, portInfo.String())
	}

	return nodePort, nil
}

func GetNodeMembersAddr(nodeObjs []*apiv1.Node) []string {
	var nodeAddr []string
	for _, node := range nodeObjs {
		addr, err := getNodeAddressForLB(node)
		if err != nil {
			// Node failure, do not create member
			klog.Warningf("failed to get node %s address: %v", node.Name, err)
			continue
		}
		nodeAddr = append(nodeAddr, addr)
	}
	return nodeAddr
}

func getNodeAddressForLB(node *apiv1.Node) (string, error) {
	addrs := node.Status.Addresses
	if len(addrs) == 0 {
		return "", errors.NewErrNodeAddressNotFound(node.Name, "")
	}

	for _, addr := range addrs {
		if addr.Type == apiv1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	return addrs[0].Address, nil
}

// NodeNames get all the node names.
func NodeNames(nodes []*apiv1.Node) []string {
	ret := make([]string, len(nodes))
	for i, node := range nodes {
		ret[i] = node.Name
	}
	return ret
}

// NodeSlicesEqual check if two nodes equals to each other.
func NodeSlicesEqual(x, y []*apiv1.Node) bool {
	if len(x) != len(y) {
		return false
	}
	return stringSlicesEqual(NodeNames(x), NodeNames(y))
}

func stringSlicesEqual(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	if !sort.StringsAreSorted(x) {
		sort.Strings(x)
	}
	if !sort.StringsAreSorted(y) {
		sort.Strings(y)
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
