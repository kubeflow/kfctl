package cmd

import (
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/kfapp/gcp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var include string
var exclude string

func init() {
	replicateImagesCmd.Flags().StringVarP(&include, "include", "i", "", "When set, only replicate images with matching prefix")
	replicateImagesCmd.Flags().StringVarP(&exclude, "exclude", "e", "gcr.io", "Skip replicate images with matching prefix")

	// verbose output
	replicateImagesCmd.Flags().BoolP(string(kftypes.VERBOSE), "V", false,
		string(kftypes.VERBOSE)+" output default is false")
	bindErr := replicateImagesCfg.BindPFlag(string(kftypes.VERBOSE), replicateImagesCmd.Flags().Lookup(string(kftypes.VERBOSE)))
	if bindErr != nil {
		log.Errorf("couldn't set flag --%v: %v", string(kftypes.VERBOSE), bindErr)
		return
	}

	alphaCmd.AddCommand(replicateImagesCmd)
}

var replicateImagesCfg = viper.New()
var replicateImagesCmd = &cobra.Command{
	Use:   "replicate-images <registry>",
	Short: "Replicate kubeflow images to target registry.",
	Long: `Replicate kubeflow images to target registry.

For kubeflow images, replicate them by image:tag -> registry/image:tag
1. rewrite images in kustomize manifests under current folder.
2. a cloud run/pod will be executed to replicate images to target registry.

`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetLevel(log.WarnLevel)
		if replicateImagesCfg.GetBool(string(kftypes.VERBOSE)) {
			log.SetLevel(log.InfoLevel)
		}
		registry := args[0]
		log.Debugf("Replicate to registry %s, include prefix %s, exclude prefix %s)\n", registry, include, exclude)
		return gcp.ReplicateToGcr(registry, include, exclude)
	},
}
