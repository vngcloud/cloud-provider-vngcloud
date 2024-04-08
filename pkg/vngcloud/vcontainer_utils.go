package vngcloud

import (
	"fmt"
	"io"

	"github.com/cuongpiger/joat/utils"
	gcfg "gopkg.in/gcfg.v1"
	lcloudProvider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	lvconSdkErr "github.com/vngcloud/vngcloud-go-sdk/error/utils"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/client"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	lvconCcmMetrics "github.com/vngcloud/cloud-provider-vngcloud/pkg/metrics"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/metadata"
	vngcloudutil "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/vngcloud"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/version"
)

// ************************************************* PUBLIC FUNCTIONS **************************************************

func NewVContainer(pCfg Config) (*VContainer, error) {
	provider, err := client.NewVContainerClient(&pCfg.Global)
	if err != nil {
		klog.Errorf("failed to init VContainer client: %v", err)
		return nil, err
	}

	metadator := metadata.GetMetadataProvider(pCfg.Metadata.SearchOrder)
	extraInfo, err := vngcloudutil.SetupPortalInfo(
		provider,
		metadator,
		utils.NormalizeURL(pCfg.Global.VServerURL)+"vserver-gateway/v1")

	if err != nil {
		klog.Errorf("failed to setup portal info: %v", err)
		return nil, err
	}

	provider.SetUserAgent(fmt.Sprintf(
		"vngcloud-controller-manager/%s (ChartVersion/%s)",
		version.Version, pCfg.Metadata.ChartVersion))

	return &VContainer{
		provider:  provider,
		vLbOpts:   pCfg.VLB,
		config:    &pCfg,
		extraInfo: extraInfo,
	}, nil
}

// ************************************************* PRIVATE FUNCTIONS *************************************************

func init() {
	// Register metrics
	lvconCcmMetrics.RegisterMetrics("vcontainer-ccm")

	// Register VNG-CLOUD cloud provider
	lcloudProvider.RegisterCloudProvider(
		consts.PROVIDER_NAME,
		func(cfg io.Reader) (lcloudProvider.Interface, error) {
			config, err := readConfig(cfg)
			if err != nil {
				klog.Warningf("failed to read config file: %v", err)
				return nil, err
			}

			config.Metadata = vngcloudutil.GetMetadataOption(config.Metadata)
			cloud, err := NewVContainer(config)
			if err != nil {
				klog.Warningf("failed to init VContainer: %v", err)
			}

			return cloud, err
		},
	)
}

func readConfig(pCfg io.Reader) (Config, error) {
	if pCfg == nil {
		return Config{}, lvconSdkErr.NewErrEmptyConfig("", "config file is empty")
	}

	// Set default
	config := Config{}

	err := gcfg.FatalOnly(gcfg.ReadInto(&config, pCfg))
	if err != nil {
		return Config{}, err
	}

	// Log the config
	klog.V(5).Infof("read config from file")
	client.LogCfg(config.Global)

	if config.Metadata.SearchOrder == "" {
		config.Metadata.SearchOrder = fmt.Sprintf("%s,%s", metadata.ConfigDriveID, metadata.MetadataID)
	}

	return config, nil
}
