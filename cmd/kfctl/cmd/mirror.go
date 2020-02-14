package cmd

import (
	"github.com/spf13/cobra"
)

// alphaCmd represents the commands that are in alpha
var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "kfctl alpha mirror",
	Long:  `kfctl alpha mirror: `,
}

func init() {
	alphaCmd.AddCommand(mirrorCmd)
}
