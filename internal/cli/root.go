package cli

import (
	"context"

	"github.com/rtgnx/vz/internal/ipc"
	"github.com/spf13/cobra"
)

func NewRootCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vz [flags]",
		Short:         "Manage virtual machines on macOS",
		SilenceUsage:  false,
		SilenceErrors: true,
	}

	cmd.AddCommand(VolumeCommand(ctx, client))
	cmd.AddCommand(NetworkCommand(ctx, client))
	cmd.AddCommand(CreateVMCommand(ctx, client))
	cmd.AddCommand(StartCommand(ctx, client))
	cmd.AddCommand(StopCommand(ctx, client))
	cmd.AddCommand(StateCommand(ctx, client))
	cmd.AddCommand(ListCommand(ctx, client))
	cmd.AddCommand(DeleteVMCommand(ctx, client))
	return cmd
}
