/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/client"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/metadata"
)

// Config struct contains ingress controller configuration
type Config struct {
	Cluster struct {
		ClusterName string `mapstructure:"clusterName"`
		ClusterID   string `mapstructure:"clusterID"`
	} `mapstructure:"cluster"`
	Kubernetes kubeConfig      `mapstructure:"kubernetes"`
	Global     client.AuthOpts `mapstructure:"global"`
	Metadata   metadata.Opts   `mapstructure:"metadata"`
}

// Configuration for connecting to Kubernetes API server, either api_host or kubeconfig should be configured.
type kubeConfig struct {
	// (Optional)Kubernetes API server host address.
	ApiserverHost string `mapstructure:"api-host"`

	// (Optional)Kubeconfig file used to connect to Kubernetes cluster.
	KubeConfig string `mapstructure:"kubeconfig"`
}
