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

package subctl

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/gather"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
)

var options gather.Options

var gatherRestConfigProducer = restconfig.NewProducer().WithContextsFlag()

var gatherCmd = &cobra.Command{
	Use:   "gather",
	Short: "Gather troubleshooting information from a cluster",
	Long: fmt.Sprintf("This command gathers information from a submariner cluster for troubleshooting. The information gathered "+
		"can be selected by component (%v) and type (%v). Default is to capture all data.",
		strings.Join(gather.AllModules.UnsortedList(), ","), strings.Join(gather.AllTypes.UnsortedList(), ",")),
	Run: func(command *cobra.Command, args []string) {
		if options.Directory == "" {
			options.Directory = "submariner-" + time.Now().UTC().Format("20060102150405") // submariner-YYYYMMDDHHMMSS
		}

		err := checkGatherArguments()
		exit.OnErrorWithMessage(err, "Invalid argument")

		status := cli.NewReporter()

		exit.OnError(gatherRestConfigProducer.RunOnAllContexts(
			func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
				return gather.Data(clusterInfo, status, options) //nolint:wrapcheck // No need to wrap errors here.
			}, status))
	},
}

func init() {
	addGatherFlags(gatherCmd)
	rootCmd.AddCommand(gatherCmd)
}

func addGatherFlags(gatherCmd *cobra.Command) {
	gatherCmd.Flags().StringSliceVar(&options.Types, "type", gather.AllTypes.UnsortedList(),
		"comma-separated list of data types to gather")
	gatherCmd.Flags().StringSliceVar(&options.Modules, "module", gather.AllModules.UnsortedList(),
		"comma-separated list of components for which to gather data")
	gatherCmd.Flags().StringVar(&options.Directory, "dir", "",
		"the directory in which to store files. If not specified, a directory of the form \"submariner-<timestamp>\" "+
			"is created in the current directory")
	gatherCmd.Flags().BoolVar(&options.IncludeSensitiveData, "include-sensitive-data", false,
		"do not redact sensitive data such as credentials and security tokens")
	gatherRestConfigProducer.SetupFlags(gatherCmd.Flags())
}

func checkGatherArguments() error {
	for _, t := range options.Types {
		if !gather.AllTypes.Has(t) {
			return fmt.Errorf("%q is not a supported type", t)
		}
	}

	for _, m := range options.Modules {
		if !gather.AllModules.Has(m) {
			return fmt.Errorf("%q is not a supported module", m)
		}
	}

	return nil
}
