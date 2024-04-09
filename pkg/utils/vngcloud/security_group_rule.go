package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/extensions/secgroup_rule"
	"k8s.io/klog/v2"
)

func CreateSecurityGroupRule(client *client.ServiceClient, projectID, secgroupID string, opts *secgroup_rule.CreateOpts) (*objects.SecgroupRule, error) {
	klog.V(5).Infoln("[API] CreateSecurityGroupRule: ", "secgroupID: ", secgroupID)
	opt := secgroup_rule.NewCreateOpts(projectID, secgroupID, opts)
	resp, err := secgroup_rule.Create(client, opt)
	return resp, err
}

func DeleteSecurityGroupRule(client *client.ServiceClient, projectID, secgroupID, secgroupRuleID string) error {
	klog.V(5).Infoln("[API] DeleteSecurityGroupRule: ", "secgroupID: ", secgroupID, "secgroupRuleID: ", secgroupRuleID)
	opt := secgroup_rule.NewDeleteOpts(projectID, secgroupID, secgroupRuleID)
	err := secgroup_rule.Delete(client, opt)
	return err
}

func ListSecurityGroupRules(client *client.ServiceClient, projectID, secgroupID string) ([]*objects.SecgroupRule, error) {
	klog.V(5).Infoln("[API] ListSecurityGroupRules: ", "secgroupID: ", secgroupID)
	opt := new(secgroup_rule.ListRulesBySecgroupIDOpts)
	opt.ProjectID = projectID
	opt.SecgroupUUID = secgroupID
	resp, err := secgroup_rule.ListRulesBySecgroupID(client, opt)
	return resp, err
}
