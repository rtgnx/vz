package config

type GuestConfig struct {
	Hostname   string
	WorkingDir string
	Env        []string
	Entrypoint []string
	SSHPubKeys []string
}
