package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/subnet"
	"k8s.io/klog/v2"
)

func GetSubnet(client *client.ServiceClient, projectID, networkID, subnetID string) (*objects.Subnet, error) {
	klog.V(5).Infoln("[API] GetSubnet: ", "networkID: ", networkID, "subnetID: ", subnetID)
	opt := subnet.NewGetOpts(projectID, networkID, subnetID)
	resp, err := subnet.Get(client, opt)
	return resp, err
}
