package config

import (
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

func Read[T any](fp string) (*T, error) {
	b, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}

	cfg := new(T)
	return cfg, yaml.Unmarshal(b, cfg)
}

func Write[T any](cfg T, fp string) error {
	b, err := yaml.Marshal(cfg)

	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fp), 0750); err != nil && !os.IsExist(err) {
		return err
	}

	return os.WriteFile(fp, b, 0640)
}
