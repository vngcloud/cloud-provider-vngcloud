package client

import (
	"github.com/cuongpiger/joat/utils"
	"github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud"
	"k8s.io/klog/v2"
)

func LogCfg(pAuthOpts AuthOpts) {
	klog.V(5).Infof("Init client with config: %+v", pAuthOpts)
}

func NewVContainerClient(authOpts *AuthOpts) (*client.ProviderClient, error) {
	identityUrl := utils.NormalizeURL(authOpts.IdentityURL) + "v2"
	provider, _ := vngcloud.NewClient(identityUrl)
	err := vngcloud.Authenticate(provider, authOpts.ToOAuth2Options())

	return provider, err
}
