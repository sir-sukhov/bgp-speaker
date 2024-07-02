package cmd

import (
	"fmt"
	"os"

	"github.com/sir-sukhov/bgp-speaker/internal/netlink"
	"github.com/spf13/cobra"
)

var (
	fibCmd = &cobra.Command{
		Use:   "fib",
		Short: "Work with routing table",
		Long:  `This command similar to 'iproute2', was added just to play around with netlink`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := netlink.PrintRoutes(); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
		},
	}
	gateway            string
	setDefaultRouteCmd = &cobra.Command{
		Use:   "set-default-route",
		Short: "Update default route to gateway",
		Long:  `This is like templated 'ip route add...'`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := netlink.SetDefaultRoute(gateway); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
		},
	}
	deleteDefaultRouteCmd = &cobra.Command{
		Use:   "delete-default-route",
		Short: "Delete default route to gateway",
		Long:  `This is like templated 'ip route del...'`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := netlink.DeleteDefaultRoute(); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
		},
	}
)

const gatewayFlagName = "gateway"

func init() {
	setDefaultRouteCmd.Flags().StringVarP(&gateway, gatewayFlagName, "g", "", "IP address of default gateway")
	_ = setDefaultRouteCmd.MarkFlagRequired(gatewayFlagName)
	fibCmd.AddCommand(setDefaultRouteCmd)
	fibCmd.AddCommand(deleteDefaultRouteCmd)
	rootCmd.AddCommand(fibCmd)
}
