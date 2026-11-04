package discovery

import (
	"context"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/enbility/zeroconf/v3"
)

func DiscoverVNC(ctx context.Context, name string) (string, error) {
	entries := make(chan *zeroconf.ServiceEntry)
	removed := make(chan *zeroconf.ServiceEntry)

	result := make(chan string, 1)

	go func() {
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					return
				}

				if entry.Instance == name {
					result <- net.JoinHostPort(
						strings.TrimSuffix(entry.HostName, "."),
						strconv.Itoa(entry.Port),
					)
					return
				}

			case entry, ok := <-removed:
				if ok {
					log.Printf("VNC disappeared: %s", entry.Instance)
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		_ = zeroconf.Browse(
			ctx,
			"_rfb._tcp",
			"local.",
			entries,
			removed,
		)
	}()

	select {
	case address := <-result:
		return address, nil

	case <-ctx.Done():
		return "", ctx.Err()
	}
}
