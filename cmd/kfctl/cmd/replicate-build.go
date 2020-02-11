package cmd

import (
	"fmt"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/gcp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var include string
var exclude string
var context string
var outputFileName string

func init() {
	replicateBuildCmd.Flags().StringVarP(&include, "include", "i", "", "When set, only replicate images with matching prefix")
	replicateBuildCmd.Flags().StringVarP(&exclude, "exclude", "e", "gcr.io", "Skip replicate images with matching prefix")
	replicateBuildCmd.Flags().StringVarP(&context, "context", "c", "", "Path to build context, for example gs://...")
	replicateBuildCmd.Flags().StringVarP(&outputFileName, "output", "o", "",
		`Name of the output pipeline file
		kfctl alpha replicate-build -o <name>`)
	// verbose output
	replicateBuildCmd.Flags().BoolP(string(kftypes.VERBOSE), "V", false,
		string(kftypes.VERBOSE)+" output default is false")
	bindErr := replicateBuildCfg.BindPFlag(string(kftypes.VERBOSE), replicateBuildCmd.Flags().Lookup(string(kftypes.VERBOSE)))
	if bindErr != nil {
		log.Errorf("couldn't set flag --%v: %v", string(kftypes.VERBOSE), bindErr)
		return
	}

	alphaCmd.AddCommand(replicateBuildCmd)
}

var replicateBuildCfg = viper.New()
var replicateBuildCmd = &cobra.Command{
	Use:   "replicate-build <registry>",
	Short: "Generate tekton pipeline file which will replicate images to target registry.",
	Long: `Generate tekton pipeline file which replicate images to target registry.

For kubeflow images, replicate them by image:tag -> registry/image:tag
a takton pipeline will be generated to replicate images to target registry.

`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetLevel(log.WarnLevel)
		if replicateBuildCfg.GetBool(string(kftypes.VERBOSE)) {
			log.SetLevel(log.InfoLevel)
		}
		if outputFileName == "" || context == "" {
			return fmt.Errorf("Please specify output file name by -o; and specify build context by -c")
		}
		registry := args[0]
		log.Debugf("Replicate to registry %s, include prefix %s, exclude prefix %s)\n", registry, include, exclude)
		return gcp.GenerateReplicationPipeline(registry, context, include, exclude, outputFileName)
	},
}
