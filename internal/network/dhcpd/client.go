//go:build linux

package dhcpd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"

	"github.com/vishvananda/netlink"
)

func DHCPClient(ctx context.Context) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	for _, link := range links {
		attrs := link.Attrs()

		if !strings.HasPrefix(attrs.Name, "eth") {
			continue
		}

		slog.Info("[netlink] link", "name", attrs.Name, "hw", attrs.HardwareAddr.String())
		c, cancel := context.WithCancel(ctx)
		wg.Add(1)
		go linkSetup(c, link, &wg)
		defer cancel()
	}

	wg.Wait()
	return nil
}

func linkSetup(ctx context.Context, link netlink.Link, wg *sync.WaitGroup) {

	defer wg.Done()

	attrs := link.Attrs()
	slog.Info("[netlink] link up", "name", attrs.Name)

	if err := netlink.LinkSetUp(link); err != nil {
		slog.Error("[netlink] link up", "name", attrs.Name, "err", err)
		return
	}

	client, err := nclient4.New(attrs.Name)

	if err != nil {
		slog.Error("[nclient] nclient setup", "link", attrs.Name, "err", err)
	}

	defer client.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			lease, err := client.Request(ctx)
			if err != nil {
				slog.Error("[nclient] dhcp lease request", "link", attrs.Name, "err", err)
				continue
			}

			ip := lease.ACK.YourIPAddr
			mask := lease.ACK.Options[1]
			if len(mask) != net.IPv4len {
				slog.Error("[nclient] DHCP ACK did not include valid IPv4", "link", attrs.Name, "ip", ip, "mask", mask)
				continue
			}

			ones, _ := net.IPMask(mask).Size()
			addr, err := netlink.ParseAddr(fmt.Sprintf("%s/%d", ip, ones))
			if err != nil {
				slog.Error("[netlink] parse address", "link", attrs.Name, "ip", ip.String())
				continue
			}

			if err := netlink.AddrReplace(link, addr); err != nil {
				slog.Error("[netlink] ip addr add", "addr", addr.String(), "link", attrs.Name)
				continue
			}

			slog.Info("[netlink] dhcp lease applied", "link", attrs.Name, "addr", addr.String())
			return
		}
	}

}
