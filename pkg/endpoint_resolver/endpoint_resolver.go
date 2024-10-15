package endpoint_resolver

import (
	"fmt"
	"slices"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	nwv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corelisters "k8s.io/client-go/listers/core/v1"
)

var ErrNotFound = errors.New("backend not found")
var ErrNodeDoesNotHaveInternalAddress = errors.New("node does not have internal address")

// An endpoint provided by pod directly.
type EndpointAddress struct {
	Name string
	IP   string
	Port int
}

// EndpointResolver resolves the endpoints for specific service & service Port.
type EndpointResolver interface {
	// convert Service Backend to int or string
	ServiceBackendToIntOrString(port nwv1.ServiceBackendPort) intstr.IntOrString
	// GetListTargetPort returns the list of target ports for the service.
	GetListTargetPort(svcKey types.NamespacedName, port intstr.IntOrString) ([]int, error)
	ResolvePodEndpoints(svcKey types.NamespacedName, port intstr.IntOrString) ([]EndpointAddress, error)
	ResolveNodePortEndpoints(svcKey types.NamespacedName, port intstr.IntOrString, nodes []*corev1.Node) ([]EndpointAddress, error)
}

// NewDefaultEndpointResolver constructs new defaultEndpointResolver
func NewDefaultEndpointResolver(serviceLister corelisters.ServiceLister, endpointLister corelisters.EndpointsLister) *defaultEndpointResolver {
	return &defaultEndpointResolver{
		serviceLister:  serviceLister,
		endpointLister: endpointLister,
	}
}

var _ EndpointResolver = &defaultEndpointResolver{}

// default implementation for EndpointResolver
type defaultEndpointResolver struct {
	serviceLister  corelisters.ServiceLister
	endpointLister corelisters.EndpointsLister
}

func (r *defaultEndpointResolver) ServiceBackendToIntOrString(port nwv1.ServiceBackendPort) intstr.IntOrString {
	if port.Name != "" {
		return intstr.FromString(port.Name)
	}
	return intstr.FromInt(int(port.Number))
}

func (r *defaultEndpointResolver) GetListTargetPort(svcKey types.NamespacedName, port intstr.IntOrString) ([]int, error) {
	svc, svcPort, err := r.findServiceAndServicePort(svcKey, port)
	if err != nil {
		return nil, err
	}

	var ports []int
	endpoints, err := r.endpointLister.Endpoints(svc.Namespace).Get(svc.Name)
	if err != nil {
		return nil, err
	}
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name && !slices.Contains(ports, int(port.Port)) {
					ports = append(ports, int(port.Port))
				}
			}
		}

		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name && !slices.Contains(ports, int(port.Port)) {
					ports = append(ports, int(port.Port))
				}
			}
		}
	}

	return ports, nil
}

func (r *defaultEndpointResolver) ResolvePodEndpoints(svcKey types.NamespacedName, port intstr.IntOrString) ([]EndpointAddress, error) {
	svc, svcPort, err := r.findServiceAndServicePort(svcKey, port)
	if err != nil {
		return nil, err
	}

	endpoints, err := r.endpointLister.Endpoints(svc.Namespace).Get(svc.Name)
	if err != nil {
		return nil, err
	}

	var podEndpoints []EndpointAddress
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name {
					podEndpoints = append(podEndpoints, EndpointAddress{
						IP:   addr.IP,
						Port: int(port.Port),
						Name: addr.TargetRef.Name,
					})
				}
			}
		}

		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name {
					podEndpoints = append(podEndpoints, EndpointAddress{
						IP:   addr.IP,
						Port: int(port.Port),
						Name: addr.TargetRef.Name,
					})
				}
			}
		}
	}

	return podEndpoints, nil
}

func (r *defaultEndpointResolver) ResolveNodePortEndpoints(svcKey types.NamespacedName, port intstr.IntOrString, nodes []*corev1.Node) ([]EndpointAddress, error) {
	svc, svcPort, err := r.findServiceAndServicePort(svcKey, port)
	if err != nil {
		return nil, err
	}
	if svc.Spec.Type != corev1.ServiceTypeNodePort && svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return nil, errors.Errorf("service type must be either 'NodePort' or 'LoadBalancer': %v", svcKey)
	}
	svcNodePort := svcPort.NodePort
	var endpoints []EndpointAddress
	for _, node := range nodes {
		nodeIP, err := r.getNodeInternalIP(node)
		if err != nil {
			continue
		}
		endpoints = append(endpoints, r.buildNodePortEndpoint(nodeIP, node.Name, svcNodePort))
	}
	return endpoints, nil
}

func (r *defaultEndpointResolver) findServiceAndServicePort(svcKey types.NamespacedName, port intstr.IntOrString) (*corev1.Service, corev1.ServicePort, error) {
	svc, err := r.serviceLister.Services(svcKey.Namespace).Get(svcKey.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, corev1.ServicePort{}, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
		}
		return nil, corev1.ServicePort{}, err
	}

	svcPort, err := r.lookupServicePort(svc, port)
	if err != nil {
		return nil, corev1.ServicePort{}, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
	}

	return svc, svcPort, nil
}

// lookupServicePort returns the ServicePort structure for specific port on service.
func (r *defaultEndpointResolver) lookupServicePort(svc *corev1.Service, port intstr.IntOrString) (corev1.ServicePort, error) {
	if port.Type == intstr.String {
		for _, p := range svc.Spec.Ports {
			if p.Name == port.StrVal {
				return p, nil
			}
		}
	} else {
		for _, p := range svc.Spec.Ports {
			if p.Port == port.IntVal {
				return p, nil
			}
		}
	}

	return corev1.ServicePort{}, errors.Errorf("unable to find port %s on service %s", port.String(), namespacedName(svc))
}

func (r *defaultEndpointResolver) buildNodePortEndpoint(IP, instanceID string, nodePort int32) EndpointAddress {
	return EndpointAddress{
		Name: instanceID,
		Port: int(nodePort),
		IP:   IP,
	}
}

func (r *defaultEndpointResolver) getNodeInternalIP(node *corev1.Node) (string, error) {
	addrs := node.Status.Addresses
	if len(addrs) == 0 {
		return "", ErrNodeDoesNotHaveInternalAddress
	}

	for _, addr := range addrs {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	return "", ErrNodeDoesNotHaveInternalAddress
}

// namespacedName returns the namespaced name for k8s objects
func namespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}
