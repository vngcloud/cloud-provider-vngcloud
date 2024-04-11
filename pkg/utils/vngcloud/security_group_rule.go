package vngcloud

import (
	"strings"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
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

func CompareSecgroupRule(current []*objects.SecgroupRule, secgroupRules []*utils.SecGroupRuleExpander) ([]string, []*utils.SecGroupRuleExpander) {
	currentIngress := make([]*objects.SecgroupRule, 0)
	for _, rule := range current {
		if strings.EqualFold(string(rule.Direction), string(secgroup_rule.CreateOptsDirectionOptIngress)) {
			currentIngress = append(currentIngress, rule)
		}
	}

	needDelete := make([]string, 0)
	needCreate := make([]*utils.SecGroupRuleExpander, 0)

	isInUse := make(map[string]bool)
	for _, rule := range currentIngress {
		isInUse[rule.UUID] = false
	}
	for _, rule := range secgroupRules {
		found := false
		for _, secgroup := range currentIngress {
			if rule.Description == secgroup.Description &&
				strings.EqualFold(string(rule.Direction), secgroup.Direction) &&
				strings.EqualFold(string(rule.EtherType), secgroup.EtherType) &&
				rule.PortRangeMax == secgroup.PortRangeMax &&
				rule.PortRangeMin == secgroup.PortRangeMin &&
				strings.EqualFold(string(rule.Protocol), secgroup.Protocol) &&
				rule.RemoteIPPrefix == secgroup.RemoteIPPrefix {
				found = true
				isInUse[secgroup.UUID] = true
				break
			}
		}
		if !found {
			needCreate = append(needCreate, rule)
		}
	}
	for _, rule := range currentIngress {
		if !isInUse[rule.UUID] {
			needDelete = append(needDelete, rule.UUID)
		}
	}
	return needDelete, needCreate
}
