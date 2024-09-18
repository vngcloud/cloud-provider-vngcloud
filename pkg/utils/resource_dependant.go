package utils

import (
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ResourceDependant interface {
	SetService(service *corev1.Service, isAddEndpoint bool)
	SetIngress(ingress *networkv1.Ingress, isAddEndpoint bool)

	GetServiceNeedReconcile(kind, namespace, resource string) []reconcile.Request
	GetIngressNeedReconcile(kind, namespace, resource string) []reconcile.Request

	ClearService(namespace, name string)
	ClearIngress(namespace, name string)
}

func NewResourceDependant() ResourceDependant {
	return &resourceDependant{
		serviceDependResources: make(map[string][]string),
		ingressDependResources: make(map[string][]string),
	}
}

type resourceDependant struct {
	serviceDependResources map[string][]string
	ingressDependResources map[string][]string
}

func (r *resourceDependant) SetService(service *corev1.Service, isAddEndpoint bool) {
	r.ClearService(service.Namespace, service.Name)
	namespace := service.Namespace
	name := service.Name
	key := namespace + "/" + name
	if isAddEndpoint {
		endpointKey := "endpoint/" + key
		r.serviceDependResources[key] = append(r.serviceDependResources[key], endpointKey)
	}

	logrus.Infof("SetService: %v", r.serviceDependResources)
}

func (r *resourceDependant) GetServiceNeedReconcile(kind, namespace, resource string) []reconcile.Request {
	if kind != "endpoint" {
		return nil
	}
	result := []reconcile.Request{}
	key := kind + "/" + namespace + "/" + resource
	for k, v := range r.serviceDependResources {
		for _, d := range v {
			if d == key {
				namespace, name := revertKey(k)
				result = append(result, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Namespace: namespace,
						Name:      name,
					},
				})
			}
		}
	}
	return result
}

func (r *resourceDependant) ClearService(namespace, name string) {
	key := namespace + "/" + name
	delete(r.serviceDependResources, key)
}

///////////////////////////////////////////

func (r *resourceDependant) SetIngress(ingress *networkv1.Ingress, isAddEndpoint bool) {
	r.ClearIngress(ingress.Namespace, ingress.Name)
	namespace := ingress.Namespace
	name := ingress.Name
	key := namespace + "/" + name
	if ingress.Spec.DefaultBackend != nil {
		serviceKey := "service/" + namespace + "/" + ingress.Spec.DefaultBackend.Service.Name
		r.ingressDependResources[key] = append(r.ingressDependResources[key], serviceKey)
		if isAddEndpoint {
			endpointKey := "endpoint/" + namespace + "/" + ingress.Spec.DefaultBackend.Service.Name
			r.ingressDependResources[key] = append(r.ingressDependResources[key], endpointKey)
		}
	}
	for _, rule := range ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {
			serviceKey := "service/" + namespace + "/" + path.Backend.Service.Name
			r.ingressDependResources[key] = append(r.ingressDependResources[key], serviceKey)
			if isAddEndpoint {
				endpointKey := "endpoint/" + namespace + "/" + path.Backend.Service.Name
				r.ingressDependResources[key] = append(r.ingressDependResources[key], endpointKey)
			}
		}
	}
	logrus.Infof("SetIngress: %v", r.ingressDependResources)
}

func (r *resourceDependant) GetIngressNeedReconcile(kind, namespace, resource string) []reconcile.Request {
	if kind != "endpoint" && kind != "service" {
		return nil
	}
	result := []reconcile.Request{}
	key := kind + "/" + namespace + "/" + resource
	for k, v := range r.ingressDependResources {
		for _, d := range v {
			if d == key {
				namespace, name := revertKey(k)
				result = append(result, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Namespace: namespace,
						Name:      name,
					},
				})
			}
		}
	}
	return result
}

func (r *resourceDependant) ClearIngress(namespace, name string) {
	key := namespace + "/" + name
	delete(r.ingressDependResources, key)
}
