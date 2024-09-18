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

func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
func EnsureNodesInCluster(pserver []*lObjects.Server) (string, []string, error) {
	networkIDs := make(map[string]bool)
	subnetIDs := make(map[string]bool)

	if len(pserver) == 0 {
		return "", []string{}, vErrors.ErrNodesAreNotSameSubnet
	}

	for _, server := range pserver {
		for _, subnet := range server.InternalInterfaces {
			networkIDs[subnet.NetworkUuid] = true
			subnetIDs[subnet.SubnetUuid] = true
		}
	}

	if len(networkIDs) != 1 || len(subnetIDs) < 1 {
		return "", nil, vErrors.ErrNodesAreNotSameSubnet
	}

	klog.Infof("EnsureNodesInCluster; networkIDs: %v, subnetIDs: %v", networkIDs, subnetIDs)

	return getKeys(networkIDs)[0], getKeys(subnetIDs), nil
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
