/***********************************************************************************************************************
 * Author: Cuong. Duong Manh <cuongdm8499@gmail.com>
 * Description:
 *  - TODO
 **/

package main

import (
	goflag "flag"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/version"
	_ "github.com/vngcloud/cloud-provider-vngcloud/pkg/vngcloud"
)

func main() {
	// Create the external controller manager with default configuration
	ccmOpts, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("Unable to initialize command options [%v]", err)
	}

	fss := cliflag.NamedFlagSets{}
	command := app.NewCloudControllerManagerCommand(
		ccmOpts,
		cloudInitializer,
		app.DefaultInitFuncConstructors,
		names.CCMControllerAliases(),
		fss,
		wait.NeverStop)

	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

	klog.V(1).Infof("The vcontainer-ccm version is [%s]", version.Version)

	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cloudInitializer(pCfg *config.CompletedConfig) cloudprovider.Interface {
	// Get configuration for CloudProvider related features.
	cloudCfg := pCfg.ComponentConfig.KubeCloudShared.CloudProvider

	// Initialize cloud provider with the cloud provider name and config file path provided

	cloud, err := cloudprovider.InitCloudProvider(cloudCfg.Name, cloudCfg.CloudConfigFile)
	klog.Info("The cloud provider name is: ", cloudCfg.Name)
	klog.Info("The cloud provider config file path is: ", cloudCfg.CloudConfigFile)
	if err != nil {
		klog.Fatalf("Failed to initialize cloud provider: [%v]", err)
	}

	// Check if the cloud provider is nil
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	// Check if the cloud provider has a cluster ID
	if !cloud.HasClusterID() {
		if pCfg.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("Detected a cluster without cluster ID. A cluster ID will be required in the future. Please tag your cluster to avoid any future issues")
		} else {
			klog.Fatal("No cluster ID found, a cluster ID is required for this cloud provider to function properly, this check can bypassed by setting the allow-untagged-cloud option")
		}
	}

	return cloud
}
