package hbus

import v1 "github.com/google/go-containerregistry/pkg/v1"

type Empty struct{}

type ImageConfig struct {
	Config v1.Config
}
