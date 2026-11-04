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

func VolumeCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "volume",
		Aliases: []string{"v"},
	}

	cmd.AddCommand(CreateVolumeCommand(ctx, client))
	cmd.AddCommand(DeleteVolumeCommand(ctx, client))
	cmd.AddCommand(ListVolumeCommand(ctx, client))
	return cmd
}

func CreateVolumeCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	opts := &config.Volume{Format: "asif"}

	cmd := &cobra.Command{
		Use:     "create [flags] NAME",
		Short:   "create an ASIF volume or register an iSCSI LUN",
		Aliases: []string{"c"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			opts.Name = args[0]

			if err := client.VolumeCreate(*opts); err != nil {
				slog.Error("creating volume", "name", opts.Name, "err", err)
				return
			}

			slog.Info("volume created", "name", opts.Name, "size", opts.Size)
		},
	}

	cmd.Flags().Uint64VarP(&opts.Size, "size", "s", 1, "volume size in GB")
	cmd.Flags().StringVar(&opts.Format, "format", opts.Format, "volume format (asif or iscsi)")
	cmd.Flags().StringVar(&opts.Address, "address", "", "iSCSI portal address (host:port)")
	cmd.Flags().StringVar(&opts.Target, "target", "", "iSCSI target IQN")
	cmd.Flags().Uint64Var(&opts.LUN, "lun", 0, "iSCSI logical unit number")
	cmd.Flags().BoolVar(&opts.ReadOnly, "read-only", false, "attach the iSCSI LUN read-only")
	cmd.Flags().StringVar(&opts.CHAPUser, "chap-user", "", "iSCSI CHAP username")
	cmd.Flags().StringVar(&opts.CHAPSecret, "chap-secret", "", "iSCSI CHAP secret")
	return cmd
}

func DeleteVolumeCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [flags] NAME",
		Short:   "delete volume",
		Aliases: []string{"d"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := client.VolumeDelete(args[0]); err != nil {
				slog.Error("deleting volume", "name", args[0], "err", err)
				return
			}

			slog.Info("volume deleted", "name", args[0])
		},
	}

	return cmd
}

func ListVolumeCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "list volumes",
		Aliases: []string{"ls", "l"},
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			volumes, err := client.VolumeList()
			if err != nil {
				slog.Error("listing volumes", "err", err)
				return
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tFORMAT\tSIZE (GB)\tSIZE ON DISK (GB)")
			for _, volume := range volumes {
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", volume.Name, volume.Format, volume.SizeGB, volume.SizeOnDiskGB)
			}
			if err := w.Flush(); err != nil {
				slog.Error("writing volume list", "err", err)
			}
		},
	}

	return cmd
}
