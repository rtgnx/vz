package vm

import (
	"context"
	"errors"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/network"
	ntypes "github.com/rtgnx/vz/internal/network/types"
	"github.com/rtgnx/vz/internal/store"
	"github.com/rtgnx/vz/internal/volume"
	vtypes "github.com/rtgnx/vz/internal/volume/types"
	vz "github.com/tmc/apple/virtualization"
	"github.com/tmc/apple/x/vzkit"
	configx "github.com/tmc/apple/x/vzkit/config"
	"github.com/tmc/apple/x/vzkit/vnc"
)

type BuildContext struct {
	ctx     context.Context
	volume  *volume.Manager
	network *network.Manager
}

func NewBuildContext(ctx context.Context, volume *volume.Manager, network *network.Manager) *BuildContext {
	return &BuildContext{ctx, volume, network}
}

func New(ctx *BuildContext, name string) (vm *VM, err error) {
	fp := store.Path(name, "vm.yml")
	cfg, err := config.Read[config.VM](fp)

	if err != nil {
		return nil, err
	}

	vm = new(VM)
	vm.q = vzkit.NewQueue(cfg.Name)
	vm.name = cfg.Name
	vm.networks = make([]ntypes.Netdev, 0)
	vm.volumes = make([]vtypes.Volume, 0)
	vm.mu = sync.Mutex{}

	vm.cfg, err = vzkit.BuildLinuxVMConfig(
		vzkit.LinuxVMConfig{
			CPUs: cfg.CPUs, MemoryGB: cfg.Memory,
			RootfsMode: vzkit.NoRootfs,
			ISOPath:    cfg.Boot.ISO,
			StateDir:   store.Path(cfg.Name),
			Network: vzkit.NetworkConfig{
				Mode: vzkit.NetworkModeNone,
			},
			Headless: !cfg.Video,
		},
	)

	err = errors.Join(
		WithRD(*cfg)(ctx, vm),
		WithVolumes(cfg.Storage.Volumes...)(ctx, vm),
		WithNetworks(cfg.Networks...)(ctx, vm),
		WithDisplay(cfg.VNCPassword)(ctx, vm),
		WithInputDevices()(ctx, vm),
	)

	if err != nil {
		return nil, errors.Join(err, vm.Close())
	}

	vm.vm = vzkit.CreateVM(vm.cfg, vm.q)
	return vm, err
}

type VMConfigFunc func(ctx *BuildContext, vm *VM) error

func WithRD(cfg config.VM) VMConfigFunc {
	return func(ctx *BuildContext, vm *VM) error {
		rd, err := volume.OpenRD(cfg)
		if err != nil {
			return err
		}
		blockdev, err := vzkit.CreateBlockDeviceWithAttachment(
			rd.Attachment(),
		)

		if err != nil {
			return err
		}

		vm.volumes = append(vm.volumes, rd)

		vzkit.AppendStorageDevices(vm.cfg, blockdev.VZStorageDeviceConfiguration)
		return nil
	}
}

func WithNetworks(names ...string) VMConfigFunc {
	return func(ctx *BuildContext, vm *VM) error {
		netdevs := []vz.VZNetworkDeviceConfiguration{}
		for _, name := range names {
			net, err := ctx.network.Open(name)

			if err != nil {
				return err
			}

			netdev, err := net.Open(vm.name)

			if err != nil {
				return err
			}

			vm.networks = append(vm.networks, netdev)
			netdevs = append(netdevs, netdev.Attachment())
		}
		vm.cfg.SetNetworkDevices(netdevs)
		return nil
	}
}

func WithDisplay(vncPassword string) VMConfigFunc {
	return func(ctx *BuildContext, vm *VM) error {

		display := vzkit.DefaultLinuxDisplayConfig()
		graphics, err := vzkit.CreateVirtioGraphicsConfig(
			[]vzkit.DisplayConfig{display},
		)
		if err != nil {
			return err
		}

		vm.cfg.SetGraphicsDevices([]vz.VZGraphicsDeviceConfiguration{
			vz.VZGraphicsDeviceConfigurationFromID(graphics.ID),
		})

		vm.vnc, err = vnc.New(vnc.Config{
			Mode:        vnc.SecurityPassword,
			Password:    vncPassword,
			ServiceName: vm.name,
			Queue:       vm.q,
		})

		if err != nil {
			return err
		}

		return nil
	}
}

func WithInputDevices() VMConfigFunc {
	return func(ctx *BuildContext, vm *VM) error {
		keyboard := vz.NewVZUSBKeyboardConfiguration()
		configx.SetKeyboards(vm.cfg, keyboard)

		pointing := vz.NewVZUSBScreenCoordinatePointingDeviceConfiguration()
		configx.SetPointingDevices(
			vm.cfg,
			[]vz.IVZPointingDeviceConfiguration{pointing},
		)

		return nil
	}
}

func WithVolumes(names ...string) VMConfigFunc {
	return func(bctx *BuildContext, vm *VM) error {
		devs := []vz.VZStorageDeviceConfiguration{}

		for _, name := range names {
			volume, err := bctx.volume.Open(nil, name)
			if err != nil {
				return err
			}

			blockdev, err := vzkit.CreateBlockDeviceWithAttachment(
				volume.Attachment(),
			)

			if err != nil {
				return err
			}

			vm.volumes = append(vm.volumes, volume)
			devs = append(devs, blockdev.VZStorageDeviceConfiguration)
		}

		vzkit.AppendStorageDevices(vm.cfg, devs...)
		return nil
	}
}
