//go:build darwin

package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/ipc"
	"github.com/spf13/cobra"
)

func CreateVMCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cfg := config.VM{
		Version:     1,
		CPUs:        1,
		Memory:      1,
		Boot:        config.BootConfig{Mode: config.BootModeDisk},
		Storage:     config.StorageConfig{RDSize: 10, Volumes: []string{}},
		VNCPassword: `test`, // macOS vnc client requires password to be present this is only for
		// testing video output and debugging for now.
	}

	bootMode := string(cfg.Boot.Mode)

	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a virtual machine",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg.Boot.Mode = config.BootMode(bootMode)
			cfg.Name = args[0]

			if err := client.VMMCreate(cfg); err != nil {
				slog.Error("create virtual machine", "name", cfg.Name, "err", err)
				return
			}

			slog.Info("virtual machine created", "name", cfg.Name)
		},
	}
	cmd.SetContext(ctx)

	flags := cmd.Flags()
	flags.UintVarP(&cfg.CPUs, "cpus", "c", cfg.CPUs, "number of CPUs")
	flags.Uint64VarP(&cfg.Memory, "memory", "m", cfg.Memory, "memory in GiB")
	flags.StringVar(&bootMode, "boot-mode", bootMode, "boot mode: iso, disk, or kernel")
	flags.StringVarP(&cfg.Boot.ISO, "iso", "i", "", "installer ISO path (boot-mode iso)")
	flags.StringVarP(&cfg.Boot.Image, "image", "k", "", "conainer image ref (boot-mode container)")
	flags.StringSliceVarP(&cfg.Networks, "network", "n", nil, "attach a named network (repeatable)")
	flags.Uint64VarP(&cfg.Storage.RDSize, "disk", "d", cfg.Storage.RDSize, "root disk size in GiB")
	flags.StringSliceVarP(&cfg.Storage.Volumes, "volume", "v", cfg.Storage.Volumes, "additional disk volumes")
	flags.BoolVarP(&cfg.Video, "video", "o", false, "enable video and VNC")
	return cmd
}

func DeleteVMCommand(ctx context.Context, client *ipc.Client) *cobra.Command {

	cmd := &cobra.Command{
		Use:     "delete [flags] VM",
		Short:   "delete virtual machine",
		Aliases: []string{"destroy", "d", "rm"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := client.VMMDelete(args[0]); err != nil {
				slog.Error("deleting virtual machine", "name", args[0], "err", err)
				return
			}

			slog.Info("virtual machine deleted", "name", args[0])
		},
	}

	return cmd
}

func StartCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start [flags] VM",
		Short: "start virtual machine",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := client.VMMStart(args[0]); err != nil {
				slog.Error("vm service start", "name", args[0], "err", err)
				return
			}
			slog.Info("vm started", "name", args[0])

		},
	}

	return cmd
}

func StopCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	force := false
	cmd := &cobra.Command{
		Use:   "stop [flags] VM",
		Short: "stop virtual machine",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := client.VMMStop(args[0], force); err != nil {
				slog.Error("vm service stop", "name", args[0], "err", err)
				return
			}

			slog.Info("vm stopped", "name", args[0])
		},
	}

	flags := cmd.Flags()
	flags.BoolVarP(&force, `force`, `f`, false, `force stop virtual machine`)

	return cmd
}

func StateCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state [flags] VM",
		Short: "show virtual machine state",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			res, err := client.VMMState(args[0])
			if err != nil {
				slog.Error("vm state", "name", args[0], "err", err)
				return
			}

			slog.Info("vm state", "name", args[0], "state", res.State.String())
		},
	}

	return cmd
}

func ListCommand(ctx context.Context, client *ipc.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "list virtual machines",
		Aliases: []string{"ls", "l"},
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			vms, err := client.VMMList()

			if err != nil {
				slog.Error("failed to fetch vm list", "err", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCPUs\tMEM\tVNC\tSTATE")

			for _, vm := range vms {
				fmt.Fprintf(w,
					"%s\t%d\t%d\t%s\t%s\n",
					vm.Name, vm.CPUs, vm.Memory, vm.VNCAddr,
					strings.TrimPrefix(vm.State.String(), "VirtualMachineState"),
				)
			}

			w.Flush()

		},
	}

	return cmd
}
