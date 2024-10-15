package endpoint_resolver

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corelisters "k8s.io/client-go/listers/core/v1"
)

type mockServiceLister struct {
	services map[types.NamespacedName]*corev1.Service
}

func (m *mockServiceLister) Services(namespace string) corelisters.ServiceNamespaceLister {
	return &mockServiceNamespaceLister{
		services:  m.services,
		namespace: namespace,
	}
}

func (m *mockServiceLister) List(selector labels.Selector) ([]*corev1.Service, error) {
	var result []*corev1.Service
	for _, svc := range m.services {
		if selector.Matches(labels.Set(svc.Labels)) {
			result = append(result, svc)
		}
	}
	return result, nil
}

type mockServiceNamespaceLister struct {
	services  map[types.NamespacedName]*corev1.Service
	namespace string
}

func (m *mockServiceNamespaceLister) Get(name string) (*corev1.Service, error) {
	svc, exists := m.services[types.NamespacedName{Namespace: m.namespace, Name: name}]
	if !exists {
		return nil, errors.New("service not found")
	}
	return svc, nil
}

func (m *mockServiceNamespaceLister) List(selector labels.Selector) ([]*corev1.Service, error) {
	var result []*corev1.Service
	for _, svc := range m.services {
		if selector.Matches(labels.Set(svc.Labels)) {
			result = append(result, svc)
		}
	}
	return result, nil
}

type mockEndpointsLister struct {
	endpoints map[types.NamespacedName]*corev1.Endpoints
}

func (m *mockEndpointsLister) Endpoints(namespace string) corelisters.EndpointsNamespaceLister {
	return &mockEndpointsNamespaceLister{
		endpoints: m.endpoints,
		namespace: namespace,
	}
}

func (m *mockEndpointsLister) List(selector labels.Selector) ([]*corev1.Endpoints, error) {
	var result []*corev1.Endpoints
	for _, ep := range m.endpoints {
		if selector.Matches(labels.Set(ep.Labels)) {
			result = append(result, ep)
		}
	}
	return result, nil
}

type mockEndpointsNamespaceLister struct {
	endpoints map[types.NamespacedName]*corev1.Endpoints
	namespace string
}

func (m *mockEndpointsNamespaceLister) Get(name string) (*corev1.Endpoints, error) {
	ep, exists := m.endpoints[types.NamespacedName{Namespace: m.namespace, Name: name}]
	if !exists {
		return nil, errors.New("endpoints not found")
	}
	return ep, nil
}

func (m *mockEndpointsNamespaceLister) List(selector labels.Selector) ([]*corev1.Endpoints, error) {
	var result []*corev1.Endpoints
	for _, ep := range m.endpoints {
		if selector.Matches(labels.Set(ep.Labels)) {
			result = append(result, ep)
		}
	}
	return result, nil
}

func TestResolvePodEndpoints(t *testing.T) {
	// Setup mock services and endpoints
	services := map[types.NamespacedName]*corev1.Service{
		{Namespace: "default", Name: "test-service"}: {
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80},
				},
			},
		},
	}
	endpoints := map[types.NamespacedName]*corev1.Endpoints{
		{Namespace: "default", Name: "test-service"}: {
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Endpoints"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: []corev1.EndpointAddress{
						{IP: "10.0.0.1", TargetRef: &corev1.ObjectReference{Name: "pod-1", Kind: "Pod"}},
					},
					Ports: []corev1.EndpointPort{
						{Name: "http", Port: 80},
					},
				},
			},
		},
	}

	// Create resolver
	serviceLister := &mockServiceLister{services: services}
	endpointsLister := &mockEndpointsLister{endpoints: endpoints}
	resolver := NewDefaultEndpointResolver(serviceLister, endpointsLister)

	// Test case
	svcKey := types.NamespacedName{Namespace: "default", Name: "test-service"}
	port := intstr.FromString("http")

	// Act
	endpointsResult, err := resolver.ResolvePodEndpoints(svcKey, port)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, len(endpointsResult))
	assert.Equal(t, "10.0.0.1", endpointsResult[0].IP)
	assert.Equal(t, 80, endpointsResult[0].Port)
}

func TestResolveNodePortEndpoints(t *testing.T) {
	// Setup mock services
	services := map[types.NamespacedName]*corev1.Service{
		{Namespace: "default", Name: "test-service"}: {
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{
					{Name: "http", NodePort: 30080},
				},
			},
		},
	}

	// Setup nodes
	nodes := []*corev1.Node{
		{
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeInternalIP, Address: "192.168.0.1"},
				},
			},
		},
	}

	// Create resolver
	serviceLister := &mockServiceLister{services: services}
	resolver := NewDefaultEndpointResolver(serviceLister, nil)

	// Test case
	svcKey := types.NamespacedName{Namespace: "default", Name: "test-service"}
	port := intstr.FromString("http")

	// Act
	endpointsResult, err := resolver.ResolveNodePortEndpoints(svcKey, port, nodes)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, len(endpointsResult))
	assert.Equal(t, "192.168.0.1", endpointsResult[0].IP)
	assert.Equal(t, 30080, endpointsResult[0].Port)
}

func TestFindServiceAndServicePortNotFound(t *testing.T) {
	// Setup empty mock services
	serviceLister := &mockServiceLister{services: map[types.NamespacedName]*corev1.Service{}}
	resolver := NewDefaultEndpointResolver(serviceLister, nil)

	// Test case
	svcKey := types.NamespacedName{Namespace: "default", Name: "missing-service"}
	port := intstr.FromString("http")

	// Act
	_, err := resolver.ResolvePodEndpoints(svcKey, port)

	// Assert
	assert.Error(t, err)
}

func TestGetNodeInternalIP(t *testing.T) {
	resolver := NewDefaultEndpointResolver(nil, nil)

	// Test case 1: Node has internal IP
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.1"},
			},
		},
	}
	ip, err := resolver.getNodeInternalIP(node)
	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.1", ip)

	// Test case 2: Node does not have internal IP
	nodeNoIP := &corev1.Node{
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{},
		},
	}
	_, err = resolver.getNodeInternalIP(nodeNoIP)
	assert.ErrorIs(t, err, ErrNodeDoesNotHaveInternalAddress)
}

func TestResolvePodEndpointsAnnd2(t *testing.T) {
	// Setup mock services and endpoints
	services := map[types.NamespacedName]*corev1.Service{
		{Namespace: "default", Name: "test-service"}: {
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
				},
			},
		},
	}
	endpoints := map[types.NamespacedName]*corev1.Endpoints{
		{Namespace: "default", Name: "test-service"}: {
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Endpoints"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: []corev1.EndpointAddress{
						{IP: "10.0.0.1", TargetRef: &corev1.ObjectReference{Name: "pod-1", Kind: "Pod"}},
						{IP: "11.0.0.1", TargetRef: &corev1.ObjectReference{Name: "pod-2", Kind: "Pod"}},
					},
					Ports: []corev1.EndpointPort{
						{Name: "http", Port: 80},
					},
				},
				{
					NotReadyAddresses: []corev1.EndpointAddress{
						{IP: "10.0.0.2", TargetRef: &corev1.ObjectReference{Name: "pod-3", Kind: "Pod"}},
						{IP: "11.0.0.2", TargetRef: &corev1.ObjectReference{Name: "pod-4", Kind: "Pod"}},
					},
					Ports: []corev1.EndpointPort{
						{Name: "http", Port: 90},
					},
				},
			},
		},
	}

	// Create resolver
	serviceLister := &mockServiceLister{services: services}
	endpointsLister := &mockEndpointsLister{endpoints: endpoints}
	resolver := NewDefaultEndpointResolver(serviceLister, endpointsLister)

	// Test case
	svcKey := types.NamespacedName{Namespace: "default", Name: "test-service"}
	port := intstr.FromString("http")

	// Act
	endpointsResult, err := resolver.ResolvePodEndpoints(svcKey, port)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 4, len(endpointsResult))
	assert.Contains(t, endpointsResult, EndpointAddress{IP: "10.0.0.1", Port: 80, Name: "pod-1"})
	assert.Contains(t, endpointsResult, EndpointAddress{IP: "11.0.0.1", Port: 80, Name: "pod-2"})
	assert.Contains(t, endpointsResult, EndpointAddress{IP: "10.0.0.2", Port: 90, Name: "pod-3"})
	assert.Contains(t, endpointsResult, EndpointAddress{IP: "11.0.0.2", Port: 90, Name: "pod-4"})

	listTargetPort, err := resolver.GetListTargetPort(svcKey, port)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(listTargetPort))
	assert.Contains(t, listTargetPort, 80)
	assert.Contains(t, listTargetPort, 90)
}
