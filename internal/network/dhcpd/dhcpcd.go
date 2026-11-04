package dhcpd

import (
	"context"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// DHCP is a DHCP packet handler associated with a packet file descriptor and
// cancellation context.
type DHCP struct {
	fd  *os.File
	ctx context.Context
}

// New constructs a DHCP handler that reads packet data from fd until ctx is
// canceled.
func New(ctx context.Context, fd *os.File) *DHCP { return &DHCP{fd, ctx} }

// handle processes a decoded Ethernet and UDP packet. DHCP message handling is
// not implemented yet.
func (d *DHCP) handle(eth *layers.Ethernet, udp *layers.UDP) {

}

// handleFrames decodes an Ethernet frame and passes UDP packets addressed to
// the DHCP server port to handle. Malformed and non-DHCP frames are ignored.
func (d *DHCP) handleFrames(frame []byte) {
	packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)

	ethl := packet.Layer(layers.LayerTypeEthernet)

	if ethl == nil {
		return
	}

	eth := ethl.(*layers.Ethernet)

	if eth == nil {
		return
	}

	udpl := packet.Layer(layers.LayerTypeUDP)

	if udpl == nil {
		return
	}

	if udp, ok := udpl.(*layers.UDP); ok && udp.DstPort == 67 {
		d.handle(eth, udp)
	}

}

// Serve runs the DHCP packet service. The service loop is not implemented yet.
func (s *DHCP) Serve() error { return nil }
