//go:build linux

package mdns

import (
	"strings"

	"github.com/topcheer/mdns"
	"github.com/vishvananda/netlink"
)

var (
	srv    *mdns.Server
	config mdns.Config = mdns.DefaultConfig()
)

func SetHostname(hostname string) {
	config.HostName = hostname
}

func Start() (err error) {
	config.Port = 5353 // Standard mDNS port
	links, err := netlink.LinkList()

	if err != nil {
		return err
	}

	for _, link := range links {
		if !strings.HasPrefix(link.Attrs().Name, "eth") {
			continue
		}
		config.Interfaces = append(config.Interfaces, link.Attrs().Name)
	}

	srv, err = mdns.NewServer(config)
	if err != nil {
		return err
	}

	return srv.Start()
}

func Close() error { return srv.Close() }
