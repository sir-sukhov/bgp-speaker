package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bgp-speaker command [options]",
	Short: "This application helps to setup gobgp library",
	Long:  `bgp-speaker can start gobgp daemon as native library and perform some additional operations`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
