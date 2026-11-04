package cli

import (
	"context"
	"fmt"
	"log/slog"
	"text/tabwriter"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/ipc"
	"github.com/spf13/cobra"
)

func NetworkCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "network",
		Aliases: []string{"net", "n"},
		Short:   "manage virtual networks",
	}

	cmd.AddCommand(CreateNetworkCommand(ctx, client))
	cmd.AddCommand(DeleteNetworkCommand(ctx, client))
	cmd.AddCommand(ListNetworkCommand(ctx, client))
	return cmd
}

func CreateNetworkCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	network := config.Network{}
	networkType := "switch"

	cmd := &cobra.Command{
		Use:     "create [flags] NAME",
		Short:   "create a virtual network",
		Aliases: []string{"c"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			network.Name = args[0]
			network.Type = config.NetworkType(networkType)

			if err := client.NetworkCreate(network); err != nil {
				slog.Error("creating network", "name", network.Name, "err", err)
				return
			}

			slog.Info("network created", "name", network.Name, "type", network.Type)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&networkType, "type", "t", networkType, "network type: switch or nat")
	flags.StringVar(&network.CIDR, "cidr", "", "network CIDR")
	flags.BoolVar(&network.DHCP, "dhcp", false, "enable DHCP")
	return cmd
}

func DeleteNetworkCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [flags] NAME",
		Short:   "delete a virtual network",
		Aliases: []string{"d", "del", "rm"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			if err := client.NetworkDelete(args[0]); err != nil {
				slog.Error("deleting network", "name", args[0], "err", err)
				return
			}

			slog.Info("network deleted", "name", args[0])
		},
	}

	return cmd
}

func ListNetworkCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "list virtual networks",
		Aliases: []string{"ls", "l"},
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			networks, err := client.NetworkList()
			if err != nil {
				slog.Error("listing networks", "err", err)
				return
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tCIDR\tDHCP")
			for _, network := range networks {
				fmt.Fprintf(w, "%s\t%s\t%s\t%t\n", network.Name, network.Type, network.CIDR, network.DHCP)
			}
			if err := w.Flush(); err != nil {
				slog.Error("writing network list", "err", err)
			}
		},
	}

	return cmd
}
