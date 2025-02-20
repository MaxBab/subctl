/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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

// Package rhos provides common functionality to run cloud prepare/cleanup on RHOS Clusters.
package rhos

import (
	"os"

	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/cloud-prepare/pkg/k8s"
	"github.com/submariner-io/cloud-prepare/pkg/ocp"
	"github.com/submariner-io/cloud-prepare/pkg/rhos"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/subctl/pkg/cluster"
)

type Config struct {
	DedicatedGateway bool
	Gateways         int
	InfraID          string
	Region           string
	ProjectID        string
	OcpMetadataFile  string
	CloudEntry       string
	GWInstanceType   string
}

// RunOn runs the given function on RHOS, supplying it with a cloud instance connected to RHOS and a reporter that writes to CLI.
// The functions makes sure that infraID and region are specified, and extracts the credentials from a secret in order to connect to RHOS.
func RunOn(clusterInfo *cluster.Info, config *Config, status reporter.Interface,
	function func(api.Cloud, api.GatewayDeployer, reporter.Interface) error,
) error {
	if config.OcpMetadataFile != "" {
		var err error

		config.InfraID, config.ProjectID, err = readMetadataFile(config.OcpMetadataFile)
		if err != nil {
			return status.Error(err, "Failed to read RHOS information from OCP metadata file %q", config.OcpMetadataFile)
		}

		status.Success("Obtained infra ID %q and project ID %q from OCP metadata file %q", config.InfraID,
			config.ProjectID, config.OcpMetadataFile)

		config.Region = os.Getenv("OS_REGION_NAME")

		status.Success("Obtained region %q from environment variable OS_REGION_NAME", config.Region)
	}

	status.Start("Retrieving RHOS credentials from your RHOS configuration")

	// Using RHOS default "openstack", if not specified
	if config.CloudEntry == "" {
		config.CloudEntry = "openstack"
	}

	opts := &clientconfig.ClientOpts{
		Cloud: config.CloudEntry,
	}

	providerClient, err := clientconfig.AuthenticatedClient(opts)
	if err != nil {
		return status.Error(err, "error initializing RHOS Client")
	}

	status.End()

	clientSet := clusterInfo.ClientProducer.ForKubernetes()
	k8sClientSet := k8s.NewInterface(clientSet)

	restMapper, err := util.BuildRestMapper(clusterInfo.RestConfig)
	if err != nil {
		return status.Error(err, "error creating REST mapper")
	}

	dynamicClient := clusterInfo.ClientProducer.ForDynamic()

	cloudInfo := rhos.CloudInfo{
		Client:    providerClient,
		InfraID:   config.InfraID,
		Region:    config.Region,
		K8sClient: k8sClientSet,
	}
	rhosCloud := rhos.NewCloud(cloudInfo)
	msDeployer := ocp.NewK8sMachinesetDeployer(restMapper, dynamicClient)
	gwDeployer := rhos.NewOcpGatewayDeployer(cloudInfo, msDeployer, config.ProjectID, config.GWInstanceType,
		"", config.CloudEntry, config.DedicatedGateway)

	return function(rhosCloud, gwDeployer, status)
}

func readMetadataFile(fileName string) (string, string, error) {
	var metadata struct {
		InfraID string `json:"infraID"`
		RHOS    struct {
			ProjectID string `json:"projectID"`
		} `json:"rhos"`
	}

	err := cloud.ReadMetadataFile(fileName, &metadata)

	return metadata.InfraID, metadata.RHOS.ProjectID, err //nolint:wrapcheck // No need to wrap here
}
