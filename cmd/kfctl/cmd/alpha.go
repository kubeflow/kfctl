package cmd

import (
	//"fmt"

	//kftypes "github.com/kubeflow/kfctl/v3/pkg/apis/apps"
	//"github.com/kubeflow/kfctl/v3/pkg/kfapp/coordinator"
	//log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var alphaCfg = viper.New()

// alphaCmd represents the commands that are in alpha
var alphaCmd = &cobra.Command{
	Use:   "alpha",
	Short: "Alpha kfctl features.",
	Long:  `Alpha kfctl features.`,
}

func init() {
	rootCmd.AddCommand(alphaCmd)
}
