package config

// Network describes a named network that the daemon creates before starting
// virtual machines.
type Network struct {
	Name string      `yaml:"name" json:"name"`
	Type NetworkType `yaml:"type" json:"type"`
	CIDR string      `yaml:"cidr,omitempty" json:"cidr,omitempty"`
	DHCP bool        `yaml:"dhcp,omitempty" json:"dhcp,omitempty"`
}

type NetworkConfig []Network

type NetworkType string

const (
	NetworkTypeInternal NetworkType = "internal"
	NetworkTypeNAT      NetworkType = "nat"
	NetworkTypeBridge   NetworkType = "bridge"
)
