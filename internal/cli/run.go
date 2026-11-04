package cli

//
// import (
// 	"context"
// 	"log/slog"
// 	"time"
//
// 	"github.com/rtgnx/vz/internal/vm"
// 	"github.com/spf13/cobra"
// )
//
// type RunFunc func(cmd *cobra.Command, args []string)
//
// func RunCommand(ctx context.Context) *cobra.Command {
// 	cmd := &cobra.Command{
// 		Use:  "run [flags] VM",
// 		Args: cobra.ExactArgs(1),
// 		Run:  Run(ctx),
// 	}
//
// 	return cmd
// }
//
// func Run(ctx context.Context) RunFunc {
// 	return func(cmd *cobra.Command, args []string) {
//
// 		vm, err := vm.New(ctx, args[0])
//
// 		if err != nil {
// 			slog.Error("unable to create vm", "err", err)
// 			return
// 		}
//
// 		slog.Info("starting virtual machine", "name", args[0])
//
// 		if err := vm.Start(); err != nil {
// 			slog.Error(err.Error())
// 			return
// 		}
//
// 		<-ctx.Done()
//
// 		shutdownCtx, cancel := context.WithTimeout(
// 			context.Background(),
// 			time.Second*120,
// 		)
//
// 		defer cancel()
//
// 		slog.Info("attempting graceful shutdown")
// 		if err := vm.Stop(shutdownCtx); err != nil {
// 			slog.Warn("graceful shutdown failed", "error", err)
//
// 			killCtx, killCancel := context.WithTimeout(
// 				context.Background(),
// 				10*time.Second,
// 			)
// 			defer killCancel()
//
// 			if err := vm.Kill(killCtx); err != nil {
// 				slog.Warn("force stop vm", "error", err)
// 			}
// 		}
//
// 		if err := vm.Close(); err != nil {
// 			slog.Error("releasing vm resources", "error", err)
// 		}
//
// 	}
// }
