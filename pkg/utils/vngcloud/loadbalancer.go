package vngcloud

import (
	"encoding/json"
	"github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	"k8s.io/klog/v2"
	"time"
)

func ListLBBySubnetID(client *client.ServiceClient, projectID string, subnetID string) ([]*objects.LoadBalancer, error) {
	klog.V(5).Infoln("[API] ListLBBySubnetID: ", "subnetID: ", subnetID)
	opt := &loadbalancer.ListBySubnetIDOpts{}
	opt.ProjectID = projectID
	opt.SubnetID = subnetID

	resp, err := loadbalancer.ListBySubnetID(client, opt)
	klog.V(5).Infoln("[API] ListLBBySubnetID: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func ListLB(client *client.ServiceClient, projectID string) ([]*objects.LoadBalancer, error) {
	opt := &loadbalancer.ListOpts{}
	opt.ProjectID = projectID
	resp, err := loadbalancer.List(client, opt)
	if err != nil {
		return resp, err.Error
	}
	return resp, nil
}

func GetLB(client *client.ServiceClient, projectID string, lbID string) (*objects.LoadBalancer, error) {
	klog.V(5).Infoln("[API] GetLB: ", "lbID: ", lbID)
	opt := &loadbalancer.GetOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID

	resp, err := loadbalancer.Get(client, opt)
	klog.V(5).Infoln("[API] GetLB: ", "resp: ", resp, "err: ", err)
	if err != nil {
		return resp, err.Error
	}
	return resp, nil
}

func CreateLB(client *client.ServiceClient, projectID string, lbOptions *loadbalancer.CreateOpts) (*objects.LoadBalancer, error) {
	klog.V(5).Infoln("[API] CreateLB: ", "lbOptions: ", lbOptions)
	lbOptions.ProjectID = projectID

	resp, err := loadbalancer.Create(client, lbOptions)
	klog.V(5).Infoln("[API] CreateLB: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func DeleteLB(client *client.ServiceClient, projectID string, lbID string) error {
	klog.V(5).Infoln("[API] DeleteLB: ", "lbID: ", lbID)
	opt := &loadbalancer.DeleteOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID

	var err error
	for {
		errSdk := loadbalancer.Delete(client, opt)
		if errSdk != nil && IsLoadBalancerNotReady(errSdk.Error) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}

	klog.V(5).Infoln("[API] DeleteLB: ", "err: ", err)
	return err
}

func ResizeLB(client *client.ServiceClient, projectID string, lbID, packageID string) error {
	klog.V(5).Infoln("[API] ResizeLB: ", "lbID: ", lbID, "packageID: ", packageID)
	opt := &loadbalancer.UpdateOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.PackageID = packageID

	var err error
	for {
		_, err = loadbalancer.Update(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] ResizeLB: ", "err: ", err)
	return err
}

type ErrorRespone struct {
	Message    string `json:"message"`
	ErrorCode  string `json:"errorCode"`
	StatusCode int    `json:"statusCode"`
}

func ParseError(errStr string) *ErrorRespone {
	if errStr == "" {
		return nil
	}
	e := &ErrorRespone{}
	err := json.Unmarshal([]byte(errStr), e)
	if err != nil {
		klog.Errorf("error when parse error: %s", err)
		return nil
	}
	return e
}
func IsLoadBalancerNotReady(err error) bool {
	e := ParseError(err.Error())
	if e != nil && (e.ErrorCode == "LoadBalancerNotReady" || e.ErrorCode == "ListenerNotReady") {
		return true
	}
	return false
}
