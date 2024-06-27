package cmd

import (
	"fmt"
	"os"

	"github.com/sir-sukhov/bgp-speaker/internal/speaker"
	"github.com/spf13/cobra"
)

var (
	configPath string
	logLevel   speaker.LogLevel

	gobgpCmd = &cobra.Command{
		Use:   "gobgp",
		Short: "Run gobgp daemon",
		Long:  `This command start gobgp daemon as native library and performs it's setup for anycast advertisement`,
		Run: func(cmd *cobra.Command, args []string) {
			app, err := speaker.NewAppCfg(configPath, logLevel)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Error in application initialization: %s\n", err)
				os.Exit(1)
			}
			if err := app.Run(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Exiting: %s\n", err)
				os.Exit(1)
			}
		},
	}
)

func init() {
	gobgpCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "config file (default is config.yaml)")
	gobgpCmd.Flags().VarP(&logLevel, "log-level", "l", "log level")
	rootCmd.AddCommand(gobgpCmd)
}
