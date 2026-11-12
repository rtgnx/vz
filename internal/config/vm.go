package config

// VM describes the persistent configuration for a virtual machine.
type VM struct {
	Version        uint          `yaml:"version" json:"version"`
	Name           string        `yaml:"name" json:"name"`
	CPUs           uint          `yaml:"cpus" json:"cpus"`
	Memory         uint64        `yaml:"memory" json:"memory"` // GiB
	Boot           BootConfig    `yaml:"boot" json:"boot"`
	Storage        StorageConfig `yaml:"storage" json:"storage"`
	Networks       []string      `yaml:"networks,omitempty" json:"networks,omitempty"`
	Video          bool          `yaml:"video,omitempty" json:"video,omitempty"`
	VNCPassword    string        `yaml:"vnc_password,omitempty" json:"vnc_password,omitempty"`
	OCIGuestConfig GuestConfig   `yaml:"oci" json:"oci"`
}

type BootMode string

const (
	BootModeISO    BootMode = "iso"
	BootModeDisk   BootMode = "disk"
	BootModeKernel BootMode = "kernel"
	BootModeOCI    BootMode = "oci"
)

type BootConfig struct {
	Mode        BootMode `yaml:"mode" json:"mode"`
	Image       string   `yaml:"image,omitempty" json:"image,omitempty"`
	ISO         string   `yaml:"iso,omitempty" json:"iso,omitempty"`
	Kernel      string   `yaml:"kernel,omitempty" json:"kernel,omitempty"`
	Initrd      string   `yaml:"initrd,omitempty" json:"initrd,omitempty"`
	CommandLine string   `yaml:"command_line,omitempty" json:"command_line,omitempty"`
}

type StorageConfig struct {
	RDSize  uint64   `yaml:"root_size,omitempty" json:"root_size,omitempty"`
	Volumes []string `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

type Volume struct {
	Name       string `yaml:"name" json:"name"`
	Size       uint64 `yaml:"size" json:"size"`
	Format     string `yaml:"format" json:"format"`
	Address    string `yaml:"address,omitempty" json:"address,omitempty"`
	Target     string `yaml:"target,omitempty" json:"target,omitempty"`
	LUN        uint64 `yaml:"lun,omitempty" json:"lun,omitempty"`
	ReadOnly   bool   `yaml:"read_only,omitempty" json:"read_only,omitempty"`
	CHAPUser   string `yaml:"chap_user,omitempty" json:"chap_user,omitempty"`
	CHAPSecret string `yaml:"chap_secret,omitempty" json:"chap_secret,omitempty"`
}
