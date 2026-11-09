package hbus

import v1 "github.com/google/go-containerregistry/pkg/v1"

type HostService struct {
	ImageConfig v1.Config
}

func (hs *HostService) GetImageConfig(_ Empty, res *ImageConfig) error {
	res.Config = hs.ImageConfig
	return nil
}
