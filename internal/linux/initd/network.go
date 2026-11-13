//go:build linux

package initd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/topcheer/mdns"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func Network(ctx context.Context, hostname string) error {
	slog.Info("[init:network]: setting hostname", "hostname", hostname)

	if err := unix.Sethostname([]byte(hostname)); err != nil {
		slog.Error("[init:network]: setting hostname", "hostname", hostname)
	}

	links, err := netlink.LinkList()

	if err != nil {
		return err
	}

	var (
		wg         sync.WaitGroup
		mdnsConfig = mdns.DefaultConfig()
	)

	mdnsConfig.Port = 5353

	for _, link := range links {
		attrs := link.Attrs()
		if !strings.HasPrefix(attrs.Name, "eth") {
			continue
		}
		wg.Add(1)
		tctx, cancel := context.WithTimeout(
			ctx, time.Second*30,
		)
		defer cancel()
		go linkSetup(tctx, link, &wg)

		mdnsConfig.Interfaces = append(
			mdnsConfig.Interfaces, attrs.Name,
		)
	}

	wg.Wait()

	slog.Info(
		"[init:network]: starting mdns",
		"hostname", hostname, "interfaces", mdnsConfig.Interfaces,
	)

	srv, err := mdns.NewServer(mdnsConfig)
	if err != nil {
		slog.Error("[init:network]: starting mdns", "err", err)
	} else {
		if err := srv.Start(); err != nil {
			slog.Error("[init:network]: starting mdns", "err", err)
		}
	}

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
		return
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

			if routers := lease.ACK.Router(); len(routers) > 0 {
				// Distinct metrics prevent concurrent DHCP replies on different
				// links from replacing each other's default routes.
				route := &netlink.Route{
					LinkIndex: attrs.Index,
					Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
					Gw:        routers[0],
					Priority:  100 + attrs.Index,
				}
				if err := netlink.RouteReplace(route); err != nil {
					slog.Error("[netlink] default route", "link", attrs.Name, "gateway", route.Gw, "err", err)
					continue
				}
				slog.Info("[netlink] default route applied", "link", attrs.Name, "gateway", route.Gw, "metric", route.Priority)
			}

			slog.Info("[netlink] dhcp lease applied", "link", attrs.Name, "addr", addr.String())
			return
		}
	}

}
