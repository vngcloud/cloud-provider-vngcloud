package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/policy"
	"k8s.io/klog/v2"
	"time"
)

func CreatePolicy(client *client.ServiceClient, projectID string, lbID, listenerID string, opt *policy.CreateOptsBuilder) (*lObjects.Policy, error) {
	klog.V(5).Infoln("[API] CreatePolicy: ", "lbID: ", lbID, "listenerID: ", listenerID)
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID

	var resp *lObjects.Policy
	var err error
	for {
		resp, err = policy.Create(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	// klog.V(5).Infoln("[API] CreatePolicy: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func ListPolicyOfListener(client *client.ServiceClient, projectID string, lbID, listenerID string) ([]*lObjects.Policy, error) {
	// klog.V(5).Infoln("[API] ListPolicyOfListener: ", "lbID: ", lbID, "listenerID: ", listenerID)
	opt := &policy.ListOptsBuilder{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID
	resp, err := policy.List(client, opt)
	// klog.V(5).Infoln("[API] ListPolicyOfListener: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func GetPolicy(client *client.ServiceClient, projectID string, lbID, listenerID, policyID string) (*lObjects.Policy, error) {
	// klog.V(5).Infoln("[API] GetPolicy: ", "lbID: ", lbID, "listenerID: ", listenerID, "policyID: ", policyID)
	opt := &policy.GetOptsBuilder{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID
	opt.PolicyID = policyID
	resp, err := policy.Get(client, opt)
	// klog.V(5).Infoln("[API] GetPolicy: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func UpdatePolicy(client *client.ServiceClient, projectID string, lbID, listenerID, policyID string, opt *policy.UpdateOptsBuilder) error {
	klog.V(5).Infoln("[API] UpdatePolicy: ", "lbID: ", lbID, "listenerID: ", listenerID, "policyID: ", policyID)
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID
	opt.PolicyID = policyID

	var err error
	for {
		err = policy.Update(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] UpdatePolicy: ", "err: ", err)
	return err
}

func DeletePolicy(client *client.ServiceClient, projectID string, lbID, listenerID, policyID string) error {
	klog.V(5).Infoln("[API] DeletePolicy: ", "lbID: ", lbID, "listenerID: ", listenerID, "policyID: ", policyID)
	opt := &policy.DeleteOptsBuilder{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID
	opt.PolicyID = policyID

	var err error
	for {
		err = policy.Delete(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] DeletePolicy: ", "err: ", err)
	return err
}
