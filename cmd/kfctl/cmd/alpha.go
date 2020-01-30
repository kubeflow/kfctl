package cmd

import (
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
