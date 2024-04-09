package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/compute/v2/server"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/secgroup"
	"k8s.io/klog/v2"
)

func ListSecurityGroups(client *client.ServiceClient, projectID string) ([]*objects.Secgroup, error) {
	klog.V(5).Infoln("[API] ListSecurityGroups")
	opt := &secgroup.ListOpts{}
	opt.ProjectID = projectID
	opt.Name = ""
	resp, err := secgroup.List(client, opt)
	return resp, err
}

func UpdateSecGroupsOfServer(client *client.ServiceClient, projectID string, instanceID string, secgroups []string) (*objects.Server, error) {
	klog.V(5).Infoln("[API] UpdateSecGroupsOfServer: ", "instanceID: ", instanceID, "secgroups: ", secgroups)
	opt := server.NewUpdateSecGroupsOpts(projectID, instanceID, secgroups)

	var resp *objects.Server
	var err error
	resp, err = server.UpdateSecGroups(client, opt)
	return resp, err
}

func GetSecurityGroup(client *client.ServiceClient, projectID string, secgroupID string) (*objects.Secgroup, error) {
	klog.V(5).Infoln("[API] GetSecurityGroup: ", "secgroupID: ", secgroupID)
	opt := &secgroup.GetOpts{}
	opt.ProjectID = projectID
	opt.SecgroupUUID = secgroupID
	resp, err := secgroup.Get(client, opt)
	if err != nil {
		return nil, err.Error
	}
	return resp, nil
}

func DeleteSecurityGroup(client *client.ServiceClient, projectID string, secgroupID string) error {
	klog.V(5).Infoln("[API] DeleteSecurityGroup: ", "secgroupID: ", secgroupID)
	opt := &secgroup.DeleteOpts{}
	opt.ProjectID = projectID
	opt.SecgroupUUID = secgroupID
	err := secgroup.Delete(client, opt)
	if err != nil {
		return err.Error
	}
	return nil
}

func CreateSecurityGroup(client *client.ServiceClient, projectID string, name string, description string) (*objects.Secgroup, error) {
	klog.V(5).Infoln("[API] CreateSecurityGroup: ", "name: ", name, "description: ", description)
	opt := &secgroup.CreateOpts{}
	opt.ProjectID = projectID
	opt.Name = name
	opt.Description = description
	resp, err := secgroup.Create(client, opt)
	return resp, err
}
