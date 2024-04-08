package controller

import (
	"context"

	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type secret struct {
	namespace string
	name      string
	uuid      string
	version   string
}

type SecretTracker struct {
	SecretTrackers []*secret
}

func NewSecretTracker() *SecretTracker {
	return &SecretTracker{
		SecretTrackers: make([]*secret, 0),
	}
}

func (c *SecretTracker) AddSecretTracker(namespace, name, uuid, version string) {
	for _, st := range c.SecretTrackers {
		if st.namespace == namespace && st.name == name {
			st.version = version
			st.uuid = uuid
			return
		}
	}
	c.SecretTrackers = append(c.SecretTrackers, &secret{
		namespace: namespace,
		name:      name,
		uuid:      uuid,
		version:   version,
	})
}

func (c *SecretTracker) RemoveSecretTracker(namespace, name string) {
	for i, st := range c.SecretTrackers {
		if st.namespace == namespace && st.name == name {
			c.SecretTrackers = append(c.SecretTrackers[:i], c.SecretTrackers[i+1:]...)
			return
		}
	}
}

func (c *SecretTracker) ClearSecretTracker() {
	c.SecretTrackers = make([]*secret, 0)
}

func (c *SecretTracker) CheckSecretTrackerChange(kubeClient kubernetes.Interface) bool {
	for _, st := range c.SecretTrackers {
		// check if certificate already exist
		secret, err := kubeClient.CoreV1().Secrets(st.namespace).Get(context.TODO(), st.name, apimetav1.GetOptions{})
		if err != nil {
			klog.Errorf("error when get secret in CheckSecretTrackerChange()")
			return true
		}
		version := secret.ObjectMeta.ResourceVersion
		if version != st.version {
			klog.Infoln("CheckSecretTrackerChange: ", st.namespace, st.name, st.uuid, st.version, version)
			return true
		}
	}
	return false
}
