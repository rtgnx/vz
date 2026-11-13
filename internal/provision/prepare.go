//go:build darwin

package provision

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/network"
	"github.com/rtgnx/vz/internal/oci"
	"github.com/rtgnx/vz/internal/store"
	"github.com/rtgnx/vz/internal/volume"
	"github.com/rtgnx/vz/internal/volume/types"
	"github.com/tmc/apple/foundation"
	vz "github.com/tmc/apple/virtualization"
	"github.com/tmc/apple/x/vzkit"
	vzconfig "github.com/tmc/apple/x/vzkit/config"
	"github.com/tmc/apple/x/vzkit/vnc"
)

const (
	OCIInitRD string = `oci-7.3.4.rd`
	OCIKernel string = `oci-7.3.4.kernel`
)

var (
	ErrUnsupportedBootMode = errors.New("unsupported boot mode")
)

type Lease interface {
	Close() error
}

type PrepareFunc func(cfg *config.VM, vmc *VMConfiguration) error

type Provisioner struct {
	volume     *volume.Manager
	network    *network.Manager
	imageStore *oci.ImageStore
}

type VMConfiguration struct {
	vnc    vnc.Config
	config vz.VZVirtualMachineConfiguration
	leases []Lease

	closeOnce sync.Once
	closeErr  error
}

func New(v *volume.Manager, n *network.Manager, i *oci.ImageStore) *Provisioner {
	return &Provisioner{v, n, i}
}

func (p *Provisioner) Close() error { return errors.Join(p.volume.Close(), p.network.Close()) }

func (vmc *VMConfiguration) Close() error {
	vmc.closeOnce.Do(func() {
		var errs []error
		for _, v := range slices.Backward(vmc.leases) {
			errs = append(errs, v.Close())
		}
		vmc.closeErr = errors.Join(errs...)
	})

	return vmc.closeErr
}

func (vmc *VMConfiguration) Config() vz.VZVirtualMachineConfiguration {
	return vmc.config
}

func (vmc *VMConfiguration) VNC(q *vzkit.Queue) vnc.Config {
	vmc.vnc.Queue = q
	return vmc.vnc
}

func (p *Provisioner) Prepare(cfg config.VM) (_ *VMConfiguration, err error) {
	vmc := &VMConfiguration{
		config: vz.NewVZVirtualMachineConfiguration(),
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, vmc.Close())
		}
	}()

	var steps []PrepareFunc = []PrepareFunc{
		withVNC(),
		withInput(),
		withDisplay(),
		withImage(p.imageStore),
		withRootDisk(),
		withVolumes(p.volume),
		withNetworks(p.network),
		withBootConfig(),
		withGuestConfig(),
		withSerialLogger(),
		withComputeResource(),
	}

	for i, step := range steps {
		if err := step(&cfg, vmc); err != nil {
			slog.Error(fmt.Sprintf("[%d/%d]", i, len(steps)), "err", err)
			return nil, err
		}
	}

	return vmc, nil
}

func withVNC() PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		if !cfg.Video {
			return nil
		}

		vmc.vnc = vnc.Config{
			ServiceName: cfg.Name,
			Password:    cfg.VNCPassword,
			Queue:       nil,
			Mode:        vnc.SecurityPassword,
		}

		return nil
	}
}

func withInput() PrepareFunc {

	return func(cfg *config.VM, vmc *VMConfiguration) error {
		if !cfg.Video {
			return nil
		}

		vzconfig.SetKeyboards(
			vmc.config, vz.NewVZUSBKeyboardConfiguration(),
		)

		vzconfig.SetPointingDevices(
			vmc.config,
			[]vz.IVZPointingDeviceConfiguration{
				vz.NewVZUSBScreenCoordinatePointingDeviceConfiguration(),
			},
		)

		return nil
	}
}

func withImage(store *oci.ImageStore) PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		if cfg.Boot.Mode != config.BootModeOCI {
			return nil
		}

		ref, err := name.ParseReference(cfg.Boot.Image)

		if err != nil {
			return err
		}

		config, err := store.Config(ref)
		if err != nil {
			return err
		}

		cfg.OCIGuestConfig.Env = append(config.Env, cfg.OCIGuestConfig.Env...)
		cfg.OCIGuestConfig.Hostname = cfg.Name

		switch {
		case len(cfg.OCIGuestConfig.Entrypoint) < 1:
			cfg.OCIGuestConfig.Entrypoint = config.Entrypoint
		case cfg.OCIGuestConfig.WorkingDir == "":
			cfg.OCIGuestConfig.WorkingDir = config.WorkingDir
		}

		volume, err := store.OpenRD(ref)

		if err != nil {
			return err
		}

		vmc.leases = append(vmc.leases, volume)
		dev, err := vzkit.CreateBlockDeviceWithAttachment(volume.Attachment())

		if err != nil {
			return err
		}

		vzkit.AppendStorageDevices(vmc.config, dev.VZStorageDeviceConfiguration)

		return nil
	}
}

func withVolumes(vlm *volume.Manager) PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		var storageDevConfigs []vz.VZStorageDeviceConfiguration

		for _, name := range cfg.Storage.Volumes {
			volume, err := vlm.Open(context.Background(), name)

			if err != nil {
				return err
			}

			vmc.leases = append(vmc.leases, volume)

			virtioBlockDev, err := vzkit.CreateBlockDeviceWithAttachment(
				volume.Attachment(),
			)

			if err != nil {
				return err
			}
			storageDevConfigs = append(storageDevConfigs, virtioBlockDev.VZStorageDeviceConfiguration)
		}

		vzkit.AppendStorageDevices(vmc.config, storageDevConfigs...)

		return nil
	}
}

func withDisplay() PrepareFunc {

	return func(cfg *config.VM, vmc *VMConfiguration) error {
		if !cfg.Video {
			return nil
		}

		display := vzkit.DefaultLinuxDisplayConfig()
		virtioGraphics, err := vzkit.CreateVirtioGraphicsConfig(
			[]vzkit.DisplayConfig{display},
		)

		if err != nil {
			return err
		}
		vmc.config.SetGraphicsDevices(
			[]vz.VZGraphicsDeviceConfiguration{
				vz.VZGraphicsDeviceConfigurationFromID(virtioGraphics.ID),
			},
		)

		return nil
	}
}

func withNetworks(nm *network.Manager) PrepareFunc {

	return func(cfg *config.VM, vmc *VMConfiguration) error {
		netdevs := []vz.VZNetworkDeviceConfiguration{}

		for _, name := range cfg.Networks {
			net, err := nm.Open(name)

			if err != nil {
				return err
			}

			netdev, err := net.Open(cfg.Name)
			if err != nil {
				return err
			}
			vmc.leases = append(vmc.leases, netdev)
			netdevs = append(netdevs, netdev.Attachment())
		}

		vmc.config.SetNetworkDevices(netdevs)
		return nil
	}
}

func withRootDisk() PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) (err error) {

		var rd types.Volume

		if rd, err = volume.OpenRD(*cfg); err != nil {
			return err
		}

		vmc.leases = append(vmc.leases, rd)

		dev, err := vzkit.CreateBlockDeviceWithAttachment(
			rd.Attachment(),
		)

		if err != nil {
			return err
		}

		vzkit.AppendStorageDevices(vmc.config, dev.VZStorageDeviceConfiguration)
		return nil
	}
}

func withBootConfig() PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		switch cfg.Boot.Mode {
		case config.BootModeISO, config.BootModeDisk:
			efiStorePath := foundation.NewURLFileURLWithPath(OCIKernel)
			efi := vz.NewVZEFIBootLoader()
			efi.SetVariableStore(vz.NewEFIVariableStoreWithURL(efiStorePath))
			vmc.config.SetBootLoader(efi)
			return nil
		case config.BootModeOCI:
			var (
				kernelURL = foundation.NewURLFileURLWithPath(
					store.Path(`kernel`, OCIKernel),
				)

				initrdURL = foundation.NewURLFileURLWithPath(
					store.Path(`kernel`, OCIInitRD),
				)

				loader = vz.NewLinuxBootLoaderWithKernelURL(kernelURL)
			)

			cfg.Boot.CommandLine = "console=hvc0 " + cfg.Boot.CommandLine

			loader.SetInitialRamdiskURL(initrdURL)
			loader.SetCommandLine(cfg.Boot.CommandLine)

			vmc.config.SetBootLoader(loader)
			return nil
		case config.BootModeKernel:

			var (
				kernelURL = foundation.NewURLFileURLWithPath(cfg.Boot.Kernel)
				initrdURL = foundation.NewURLFileURLWithPath(cfg.Boot.Initrd)

				loader = vz.NewLinuxBootLoaderWithKernelURL(kernelURL)
			)

			loader.SetInitialRamdiskURL(initrdURL)
			loader.SetCommandLine(cfg.Boot.CommandLine)

			vmc.config.SetBootLoader(loader)
			return nil
		default:
			return ErrUnsupportedBootMode
		}
	}
}

func withGuestConfig() PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		guestConfigPath := store.Path(cfg.Name, `guest`, `guest.yml`)

		if err := config.Write(cfg.OCIGuestConfig, guestConfigPath); err != nil {
			return err
		}

		hostURL := foundation.NewURLFileURLWithPath(
			filepath.Dir(guestConfigPath),
		)

		directory := vz.NewSharedDirectoryWithURLReadOnly(hostURL, true)
		share := vz.NewSingleDirectoryShareWithDirectory(&directory)

		device := vz.NewVirtioFileSystemDeviceConfigurationWithTag("guest-config")
		device.SetShare(&share.VZDirectoryShare)
		vmc.config.SetDirectorySharingDevices(
			append(
				vmc.config.DirectorySharingDevices(),
				device.VZDirectorySharingDeviceConfiguration,
			),
		)

		return nil
	}
}

func withSerialLogger() PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		logPath := store.Path(cfg.Name, "console.log")

		attachment, err := vz.NewFileSerialPortAttachmentWithURLAppendError(
			foundation.NewURLFileURLWithPath(logPath),
			true, // Append across VM boots.
		)

		if err != nil {
			return err
		}

		port := vz.NewVZVirtioConsoleDeviceSerialPortConfiguration()
		port.SetAttachment(attachment)
		vmc.config.SetSerialPorts(
			append(vmc.config.SerialPorts(), port.VZSerialPortConfiguration),
		)

		return nil
	}
}

func withComputeResource() PrepareFunc {
	return func(cfg *config.VM, vmc *VMConfiguration) error {
		vmc.config.SetCPUCount(cfg.CPUs)
		vmc.config.SetMemorySize(cfg.Memory << 30)
		return nil
	}
}
