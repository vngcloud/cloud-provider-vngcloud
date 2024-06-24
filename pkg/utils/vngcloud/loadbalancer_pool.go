package vngcloud

import (
	"time"

	"github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	"k8s.io/klog/v2"
)

func CreatePool(client *client.ServiceClient, projectID string, lbID string, opt *pool.CreateOpts) (*lObjects.Pool, error) {
	klog.V(5).Infoln("[API] CreatePool: ", "lbID: ", lbID, "opt: ", opt)
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID

	var resp *lObjects.Pool
	var err error
	for {
		resp, err = pool.Create(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] CreatePool: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func ListPoolOfLB(client *client.ServiceClient, projectID string, lbID string) ([]*lObjects.Pool, error) {
	klog.V(5).Infoln("[API] ListPool: ", "lbID: ", lbID)
	opt := &pool.ListPoolsBasedLoadBalancerOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID

	resp, err := pool.ListPoolsBasedLoadBalancer(client, opt)
	klog.V(5).Infoln("[API] ListPool: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func UpdatePoolMember(client *client.ServiceClient, projectID string, lbID, poolID string, mems []*pool.Member) error {
	klog.V(5).Infoln("[API] UpdatePoolMember: ", "poolID: ", poolID, "mems: ", mems)
	for _, mem := range mems {
		klog.V(5).Infof("[%s, %s, %d, %d]", mem.Name, mem.IpAddress, mem.Port, mem.MonitorPort)
	}
	opt := &pool.UpdatePoolMembersOpts{
		Members: mems,
	}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.PoolID = poolID

	var err error
	for {
		err = pool.UpdatePoolMembers(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}

	klog.V(5).Infoln("[API] UpdatePoolMember: ", "err: ", err)
	return err
}

func GetPool(client *client.ServiceClient, projectID string, lbID, poolID string) (*lObjects.Pool, error) {
	klog.V(5).Infoln("[API] GetPool: ", "poolID: ", poolID)
	opt := &pool.GetOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.PoolID = poolID

	resp, err := pool.GetTotal(client, opt)
	klog.V(5).Infoln("[API] GetPool: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func GetMemberPool(client *client.ServiceClient, projectID string, lbID, poolID string) ([]*lObjects.Member, error) {
	klog.V(5).Infoln("[API] GetMemberPool: ", "poolID: ", poolID)
	opt := &pool.GetMemberOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.PoolID = poolID

	resp, err := pool.GetMember(client, opt)
	klog.V(5).Infoln("[API] GetMemberPool: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func DeletePool(client *client.ServiceClient, projectID string, lbID, poolID string) error {
	klog.V(5).Infoln("[API] DeletePool: ", "poolID: ", poolID)
	opt := &pool.DeleteOpts{}
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.PoolID = poolID

	var err error
	for {
		errSdk := pool.Delete(client, opt)
		if errSdk != nil && IsLoadBalancerNotReady(errSdk.Error) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] DeletePool: ", "err: ", err)
	return err
}

func UpdatePool(client *client.ServiceClient, projectID string, lbID, poolID string, opt *pool.UpdateOpts) error {
	klog.V(5).Infoln("[API] UpdatePool: ", "lbID: ", lbID, "poolID: ", poolID, "opt: ", opt)
	opt.ProjectID = projectID
	opt.LoadBalancerID = lbID
	opt.PoolID = poolID

	var err error
	for {
		err = pool.Update(client, opt)
		if err != nil && IsLoadBalancerNotReady(err) {
			klog.V(5).Infof("LoadBalancerNotReady, retry after 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		} else {
			break
		}
	}
	klog.V(5).Infoln("[API] UpdatePool: ", "err: ", err)
	return err
}
