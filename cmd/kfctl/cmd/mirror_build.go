package cmd

import (
	"fmt"
	kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	mirrortypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps/imagemirror/v1alpha1"
	"github.com/kubeflow/kfctl/v3/pkg/mirror"
	"github.com/kubeflow/kfctl/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"sigs.k8s.io/yaml"
)

var outputFileName string
var directory string
var gcb bool

func init() {
	replicateBuildCmd.Flags().StringVarP(&outputFileName, "output", "o", "",
		`Name of the output pipeline file
		kfctl alpha mirror build -o <name>`)
	replicateBuildCmd.Flags().StringVarP(&directory, "directory", "d", "kustomize",
		`The directory to search for kustomization files listing images to mirror
		kfctl alpha mirror build -d <directory>`)
	replicateBuildCmd.Flags().BoolVar(&gcb, "gcb", false, `Generate cloud build config`)
	// verbose output
	replicateBuildCmd.Flags().BoolP(string(kftypes.VERBOSE), "V", false,
		string(kftypes.VERBOSE)+" output default is false")
	bindErr := replicateBuildCfg.BindPFlag(string(kftypes.VERBOSE), replicateBuildCmd.Flags().Lookup(string(kftypes.VERBOSE)))
	if bindErr != nil {
		log.Errorf("Couldn't set flag --%v: %v", string(kftypes.VERBOSE), bindErr)
		return
	}

	mirrorCmd.AddCommand(replicateBuildCmd)
}

var replicateBuildCfg = viper.New()
var replicateBuildCmd = &cobra.Command{
	Use:   "build <local_config_file_path> -o <pipeline_file>",
	Short: "Generate tekton pipeline file which will replicate images to target registry.",
	Long: `Generate tekton pipeline file which replicate images to target registry.

Image replication rules are defined in config file.

`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetLevel(log.WarnLevel)
		if replicateBuildCfg.GetBool(string(kftypes.VERBOSE)) {
			log.SetLevel(log.InfoLevel)
		}
		configFile := args[0]
		isRemoteFile, err := utils.IsRemoteFile(configFile)
		if err != nil {
			return err
		}
		if isRemoteFile {
			return fmt.Errorf("config file path should be non-empty local file.")
		}

		if outputFileName == "" {
			return fmt.Errorf("You must specify an output file with -o")
		}
		if _, err := os.Stat(configFile); err != nil {
			return err
		}
		confBytes, err := ioutil.ReadFile(configFile)
		if err != nil {
			return nil
		}
		replication := mirrortypes.Replication{}
		if err := yaml.Unmarshal(confBytes, &replication); err != nil {
			return err
		}
		for _, pattern := range replication.Spec.Patterns {
			log.Infof("Context: %v; destination registry: %v", replication.Spec.Context, pattern.Dest)
			if replication.Spec.Context == "" || pattern.Dest == "" {
				return fmt.Errorf("Config: context and dest registry cannot be empty")
			}

		}

		return mirror.GenerateMirroringPipeline(directory, replication.Spec, outputFileName, gcb)
	},
}
