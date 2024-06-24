package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"k8s.io/klog/v2"
	"time"
)

func CreateListener(client *client.ServiceClient, projectID string, lbID string, opt *listener.CreateOpts) (*lObjects.Listener, error) {
	klog.V(5).Infoln("[API] CreateListener: ", "lbID: ", lbID, "opt: ", opt)
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID

	var resp *lObjects.Listener
	var err error
	for {
		resp, err = listener.Create(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	return resp, err
}

func ListListenerOfLB(client *client.ServiceClient, projectID string, lbID string) ([]*lObjects.Listener, error) {
	klog.V(5).Infoln("[API] ListListenerOfLB: ", "lbID: ", lbID)
	opt := &listener.GetBasedLoadBalancerOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	resp, err := listener.GetBasedLoadBalancer(client, opt)
	return resp, err
}

func DeleteListener(client *client.ServiceClient, projectID string, lbID, listenerID string) error {
	klog.V(5).Infoln("[API] DeleteListener: ", "lbID: ", lbID, "listenerID: ", listenerID)
	opt := &listener.DeleteOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID

	var err error
	for {
		errSdk := listener.Delete(client, opt)
		if errSdk != nil && IsLoadBalancerNotReady(errSdk.Error) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] DeleteListener: ", "err: ", err)
	return err
}

func UpdateListener(client *client.ServiceClient, projectID string, lbID, listenerID string, opt *listener.UpdateOpts) error {
	klog.V(5).Infoln("[API] UpdateListener: ", "lbID: ", lbID, "listenerID: ", listenerID, "opt: ", opt)
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.ListenerID = listenerID

	var err error
	for {
		err = listener.Update(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] UpdateListener: ", "err: ", err)
	return err
}
