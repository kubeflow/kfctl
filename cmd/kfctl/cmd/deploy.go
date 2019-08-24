// Copyright © 2019 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"log"

	"github.com/kubeflow/kfctl/v3/pkg/kfapp/deploy"
	"github.com/spf13/cobra"
)

// ConfigURI gets the config file URI
var ConfigURI string

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploys Kubeflow.",
	Long:  `Deploys Kubeflow with a single config file as input.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("deploy called")
		err := deploy.InstallKubeflow("kf-app", ConfigURI)
		if err != nil {
			log.Printf("Error deploying Kubeflow: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringVar(&ConfigURI, "config", "https://github.com/kubeflow/kubeflow/blob/master/bootstrap/config/kfctl_k8s_istio.yaml", "Config for deploying Kubeflow")
}
