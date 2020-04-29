package cmd

import (
	"fmt"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	"github.com/kubeflow/kfctl/v3/pkg/mirror"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var inputFileName string

func init() {
	replicateOverwriteCmd.Flags().StringVarP(&inputFileName, "input", "i", "",
		`Name of the input pipeline file
		kfctl alpha  mirror overwrite -o <name>`)
	// verbose output
	replicateOverwriteCmd.Flags().BoolP(string(kftypes.VERBOSE), "V", false,
		string(kftypes.VERBOSE)+" output default is false")
	bindErr := replicateOverwriteCfg.BindPFlag(string(kftypes.VERBOSE), replicateOverwriteCmd.Flags().Lookup(string(kftypes.VERBOSE)))
	if bindErr != nil {
		log.Errorf("Couldn't set flag --%v: %v", string(kftypes.VERBOSE), bindErr)
		return
	}

	mirrorCmd.AddCommand(replicateOverwriteCmd)
}

var replicateOverwriteCfg = viper.New()
var replicateOverwriteCmd = &cobra.Command{
	Use:   "overwrite <registry>",
	Short: "",
	Long: `Read input tekton pipeline file with images replication info,
update images in kustomization.yaml: image:tag -> newImage:tag
`,
	Args: cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetLevel(log.WarnLevel)
		if replicateOverwriteCfg.GetBool(string(kftypes.VERBOSE)) {
			log.SetLevel(log.InfoLevel)
		}
		if inputFileName == "" {
			return fmt.Errorf("Please specify input tekton pipeline file by -i")
		}
		return mirror.UpdateKustomize(inputFileName)
	},
}
