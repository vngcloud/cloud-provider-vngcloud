package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/compute/v2/server"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/secgroup"
	"k8s.io/klog/v2"
)

func ListSecurityGroups(client *client.ServiceClient, projectID string) ([]*objects.Secgroup, error) {
	klog.V(5).Infoln("[API] GetSecurityGroups")
	opt := &secgroup.ListOpts{}
	opt.ProjectID = projectID
	opt.Name = ""
	resp, err := secgroup.List(client, opt)
	return resp, err
}

func UpdateSecGroups(client *client.ServiceClient, projectID string, instanceID string, secgroups []string) (*objects.Server, error) {
	klog.V(5).Infoln("[API] UpdateSecGroups: ", "instanceID: ", instanceID, "secgroups: ", secgroups)
	opt := server.NewUpdateSecGroupsOpts(projectID, instanceID, secgroups)

	var resp *objects.Server
	var err error
	resp, err = server.UpdateSecGroups(client, opt)
	return resp, err
}
