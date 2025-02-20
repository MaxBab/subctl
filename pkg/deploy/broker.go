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

package deploy

import (
	"context"
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/subctl/pkg/operator"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"k8s.io/apimachinery/pkg/util/sets"
)

type BrokerOptions struct {
	OperatorDebug   bool
	Repository      string
	ImageVersion    string
	BrokerNamespace string
	BrokerSpec      operatorv1alpha1.BrokerSpec
}

var ValidComponents = []string{component.ServiceDiscovery, component.Connectivity}

func Broker(options *BrokerOptions, clientProducer client.Producer, status reporter.Interface,
) error {
	componentSet := sets.New(options.BrokerSpec.Components...)
	ctx := context.TODO()

	if err := isValidComponents(componentSet); err != nil {
		return status.Error(err, "invalid components parameter")
	}

	if options.BrokerSpec.GlobalnetEnabled {
		componentSet.Insert(component.Globalnet)
	}

	if err := checkGlobalnetConfig(options); err != nil {
		return status.Error(err, "invalid GlobalCIDR configuration")
	}

	err := deploy(ctx, options, status, clientProducer)
	if err != nil {
		return err
	}

	if options.BrokerSpec.GlobalnetEnabled {
		if err = globalnet.ValidateExistingGlobalNetworks(ctx, clientProducer.ForGeneral(), options.BrokerNamespace); err != nil {
			return status.Error(err, "error validating existing globalCIDR configmap")
		}
	}

	if err = globalnet.CreateConfigMap(ctx, clientProducer.ForGeneral(), options.BrokerSpec.GlobalnetEnabled,
		options.BrokerSpec.GlobalnetCIDRRange, options.BrokerSpec.DefaultGlobalnetClusterSize, options.BrokerNamespace); err != nil {
		return status.Error(err, "error creating globalCIDR configmap on Broker")
	}

	return nil
}

func deploy(ctx context.Context, options *BrokerOptions, status reporter.Interface, clientProducer client.Producer) error {
	status.Start("Setting up broker RBAC")
	defer status.End()

	err := broker.Ensure(ctx, crd.UpdaterFromControllerClient(clientProducer.ForGeneral()), clientProducer.ForKubernetes(),
		options.BrokerSpec.Components, false, options.BrokerNamespace)
	if err != nil {
		return status.Error(err, "error setting up broker RBAC")
	}

	status.Start("Deploying the Submariner operator")

	repositoryInfo := image.NewRepositoryInfo(options.Repository, options.ImageVersion, nil)

	err = operator.Ensure(ctx, status, clientProducer, constants.OperatorNamespace, repositoryInfo.GetOperatorImage(), options.OperatorDebug)
	if err != nil {
		return status.Error(err, "error deploying Submariner operator")
	}

	status.Start("Deploying the broker")

	err = brokercr.Ensure(ctx, clientProducer.ForGeneral(), options.BrokerNamespace, options.BrokerSpec)

	return status.Error(err, "Broker deployment failed")
}

func isValidComponents(componentSet sets.Set[string]) error {
	validComponentSet := sets.New(ValidComponents...)

	if componentSet.Len() < 1 {
		return fmt.Errorf("at least one component must be provided for deployment")
	}

	for _, component := range componentSet.UnsortedList() {
		if !validComponentSet.Has(component) {
			return fmt.Errorf("unknown component: %s", component)
		}
	}

	return nil
}

//nolint:wrapcheck // No need to wrap errors here.
func checkGlobalnetConfig(options *BrokerOptions) error {
	var err error

	if !options.BrokerSpec.GlobalnetEnabled {
		return nil
	}

	options.BrokerSpec.DefaultGlobalnetClusterSize, err = globalnet.GetValidClusterSize(options.BrokerSpec.GlobalnetCIDRRange,
		options.BrokerSpec.DefaultGlobalnetClusterSize)
	if err != nil {
		return err
	}

	return globalnet.IsValidCIDR(options.BrokerSpec.GlobalnetCIDRRange)
}
