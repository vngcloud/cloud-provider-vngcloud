package vngcloud

import (
	"fmt"
	"strings"

	vErrors "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/errors"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/metadata"
	client2 "github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/extensions/secgroup_rule"
	lPortal "github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/portal/v1"
	"k8s.io/klog/v2"
)

type (
	ExtraInfo struct {
		ProjectID string
		UserID    int64
	}
)

func GetMetadataOption(pMetadata metadata.Opts) metadata.Opts {
	if pMetadata.SearchOrder == "" {
		pMetadata.SearchOrder = fmt.Sprintf("%s,%s", metadata.ConfigDriveID, metadata.MetadataID)
	}
	klog.Info("getMetadataOption; metadataOpts is ", pMetadata)
	return pMetadata
}

func SetupPortalInfo(pProvider *client2.ProviderClient, pMedatata metadata.IMetadata, pPortalURL string) (*ExtraInfo, error) {
	projectID, err := pMedatata.GetProjectID()
	if err != nil {
		return nil, err
	}
	klog.Infof("SetupPortalInfo; projectID is %s", projectID)

	portalClient, _ := vngcloud.NewServiceClient(pPortalURL, pProvider, "portal")
	portalInfo, err := lPortal.Get(portalClient, projectID)

	if err != nil {
		return nil, err
	}

	if portalInfo == nil {
		return nil, fmt.Errorf("can not get portal information")
	}

	return &ExtraInfo{
		ProjectID: portalInfo.ProjectID,
		UserID:    portalInfo.UserID,
	}, nil
}

func EnsureNodesInCluster(pserver []*lObjects.Server) (string, error) {
	subnetMapping := make(map[string]int)
	for _, server := range pserver {
		subnets := listSubnetIDs(server)
		for _, subnet := range subnets {
			if smi, ok := subnetMapping[subnet]; !ok {
				subnetMapping[subnet] = 1
			} else {
				subnetMapping[subnet] = smi + 1
			}
		}
	}

	for subnet, count := range subnetMapping {
		if count == len(pserver) && len(subnet) > 0 {
			return subnet, nil
		}
	}

	return "", vErrors.ErrNodesAreNotSameSubnet
}

func listSubnetIDs(s *lObjects.Server) []string {
	var subnets []string
	if s == nil {
		return subnets
	}

	for _, subnet := range s.InternalInterfaces {
		subnets = append(subnets, subnet.SubnetUuid)
	}

	return subnets
}

func GetNetworkID(pserver []*lObjects.Server, pSubnetID string) string {
	for _, server := range pserver {
		for _, subnet := range server.InternalInterfaces {
			if subnet.SubnetUuid == pSubnetID {
				return subnet.NetworkUuid
			}
		}
	}
	return ""
}

func HealthcheckProtocoToSecGroupProtocol(p string) secgroup_rule.CreateOptsProtocolOpt {
	protocol := strings.ToLower(p)
	switch protocol {
	case "tcp":
		return secgroup_rule.CreateOptsProtocolOptTCP
	case "udp":
		return secgroup_rule.CreateOptsProtocolOptUDP
	case "icmp":
		return secgroup_rule.CreateOptsProtocolOptICMP
	default:
		return secgroup_rule.CreateOptsProtocolOptTCP
	}
}
