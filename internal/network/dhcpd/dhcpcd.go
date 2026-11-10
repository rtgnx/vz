// Package dhcpd implements a small DHCPv4 server for Ethernet frames carried
// by the virtual switch.
package dhcpd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

const (
	dhcpClientPort = 68
	dhcpServerPort = 67
	dhcpLeaseTime  = 24 * 60 * 60
)

var (
	errInvalidCIDR  = errors.New("DHCP server requires an IPv4 subnet with at least two usable addresses")
	errInvalidFrame = errors.New("invalid Ethernet/IPv4/UDP frame")
)

// Server handles DHCPv4 client frames and returns replies suitable for
// forwarding through the switch. The first usable address in cidr is reserved
// for the server; client leases are allocated from the remaining host range.
type Server struct {
	cidr     net.IPNet
	serverIP net.IP
	serverHW net.HardwareAddr
	poolFrom uint32
	poolTo   uint32
	hwtable  map[string]net.IP
	mu       sync.Mutex
}

// NewServer creates a DHCP server for an IPv4 subnet. The first usable address
// is the server address and is never assigned to a client.
func NewServer(cidr net.IPNet) (*Server, error) {
	ip := cidr.IP.To4()
	mask := cidr.Mask
	if ip == nil || len(mask) != net.IPv4len {
		return nil, errInvalidCIDR
	}
	prefix, bits := mask.Size()
	if bits != 32 || prefix > 30 {
		return nil, errInvalidCIDR
	}

	network := binary.BigEndian.Uint32(ip.Mask(mask))
	last := network | ^binary.BigEndian.Uint32(mask)
	server := network + 1
	poolFrom := server + 1
	poolTo := last - 1
	if poolFrom > poolTo {
		return nil, errInvalidCIDR
	}

	serverIP := uint32IP(server)
	// Use a locally administered unicast address derived from the server IP.
	serverHW := net.HardwareAddr{0x02, 0x00, serverIP[0], serverIP[1], serverIP[2], serverIP[3]}

	return &Server{
		cidr:     net.IPNet{IP: net.IP(networkIP(network)), Mask: append(net.IPMask(nil), mask...)},
		serverIP: serverIP,
		serverHW: serverHW,
		poolFrom: poolFrom,
		poolTo:   poolTo,
		hwtable:  make(map[string]net.IP),
	}, nil
}

// ServeFrame processes one Ethernet frame. It returns nil, nil when the frame
// is not a DHCPv4 Discover or Request message; otherwise it returns a complete
// Ethernet reply frame for forwarding back to the client.
func (s *Server) ServeFrame(frame []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("nil DHCP server")
	}

	request, l2Len, err := decodeDHCPRequest(frame)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, nil
	}
	if len(request.ClientHWAddr) == 0 {
		return nil, errors.New("DHCP request has no client hardware address")
	}

	clientKey := string(request.ClientHWAddr)
	var (
		messageType dhcpv4.MessageType
		leaseIP     net.IP
	)

	s.mu.Lock()
	switch request.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		leaseIP = s.leaseForClient(clientKey)
		if leaseIP == nil {
			leaseIP = s.allocateLease(clientKey, nil)
		}
		if leaseIP == nil {
			s.mu.Unlock()
			return nil, errors.New("DHCP address pool exhausted")
		}
		messageType = dhcpv4.MessageTypeOffer
	case dhcpv4.MessageTypeRequest:
		// In the SELECTING state, a client broadcasts requests for one server's
		// offer. Other servers must remain silent.
		if serverID := request.GetOneOption(dhcpv4.OptionServerIdentifier); len(serverID) == net.IPv4len && !net.IP(serverID).Equal(s.serverIP) {
			s.mu.Unlock()
			return nil, nil
		}

		requested := net.IP(request.GetOneOption(dhcpv4.OptionRequestedIPAddress)).To4()
		if requested == nil && request.ClientIPAddr != nil {
			requested = request.ClientIPAddr.To4()
		}
		leaseIP = s.leaseForClient(clientKey)
		if requested != nil && !requested.Equal(leaseIP) {
			leaseIP = s.allocateLease(clientKey, requested)
		}
		if leaseIP == nil {
			// NAK communicates that the requested address is unavailable or
			// outside this subnet's lease pool.
			messageType = dhcpv4.MessageTypeNak
		} else {
			messageType = dhcpv4.MessageTypeAck
		}
	default:
		s.mu.Unlock()
		return nil, nil
	}
	s.mu.Unlock()

	if leaseIP == nil {
		leaseIP = net.IPv4zero
	}
	return encodeDHCPReply(request, messageType, s.serverHW, s.serverIP, leaseIP, s.cidr.Mask, frame[:l2Len])
}

// leaseForClient returns the existing address assigned to clientKey.
// The caller must hold s.mu.
func (s *Server) leaseForClient(clientKey string) net.IP {
	if ip := s.hwtable[clientKey]; ip != nil {
		return append(net.IP(nil), ip...)
	}
	return nil
}

// allocateLease reserves requested for clientKey when it is available, or
// picks the first free address in the pool. The caller must hold s.mu.
func (s *Server) allocateLease(clientKey string, requested net.IP) net.IP {
	if requested != nil {
		value := binary.BigEndian.Uint32(requested.To4())
		if value < s.poolFrom || value > s.poolTo || s.addressInUse(clientKey, requested) {
			return nil
		}
		s.hwtable[clientKey] = append(net.IP(nil), requested.To4()...)
		return append(net.IP(nil), requested.To4()...)
	}

	for value := s.poolFrom; value <= s.poolTo; value++ {
		ip := uint32IP(value)
		if !s.addressInUse(clientKey, ip) {
			s.hwtable[clientKey] = ip
			return append(net.IP(nil), ip...)
		}
		if value == ^uint32(0) {
			break
		}
	}
	return nil
}

// addressInUse reports whether another client owns ip. The caller must hold
// s.mu.
func (s *Server) addressInUse(clientKey string, ip net.IP) bool {
	for key, leased := range s.hwtable {
		if key != clientKey && leased.Equal(ip) {
			return true
		}
	}
	return false
}

// decodeDHCPRequest extracts a DHCPv4 message and its Ethernet header length.
// A non-DHCP frame returns (nil, 0, nil).
func decodeDHCPRequest(frame []byte) (*dhcpv4.DHCPv4, int, error) {
	if len(frame) < 14 {
		return nil, 0, errInvalidFrame
	}

	etherType := binary.BigEndian.Uint16(frame[12:14])
	l2Len := 14
	for etherType == 0x8100 || etherType == 0x88a8 || etherType == 0x9100 {
		if len(frame) < l2Len+4 {
			return nil, 0, errInvalidFrame
		}
		etherType = binary.BigEndian.Uint16(frame[l2Len+2 : l2Len+4])
		l2Len += 4
	}
	if etherType != 0x0800 {
		return nil, 0, nil
	}
	if len(frame) < l2Len+20 || frame[l2Len]>>4 != 4 {
		return nil, 0, errInvalidFrame
	}

	ipHeaderLen := int(frame[l2Len]&0x0f) * 4
	if ipHeaderLen < 20 || len(frame) < l2Len+ipHeaderLen {
		return nil, 0, errInvalidFrame
	}
	ipLen := int(binary.BigEndian.Uint16(frame[l2Len+2 : l2Len+4]))
	if ipLen < ipHeaderLen+8 || l2Len+ipLen > len(frame) {
		return nil, 0, errInvalidFrame
	}
	if frame[l2Len+9] != 17 { // UDP
		return nil, 0, nil
	}
	fragment := binary.BigEndian.Uint16(frame[l2Len+6 : l2Len+8])
	if fragment&0x1fff != 0 {
		return nil, 0, nil
	}
	udpOffset := l2Len + ipHeaderLen
	if binary.BigEndian.Uint16(frame[udpOffset:udpOffset+2]) != dhcpClientPort ||
		binary.BigEndian.Uint16(frame[udpOffset+2:udpOffset+4]) != dhcpServerPort {
		return nil, 0, nil
	}
	udpLen := int(binary.BigEndian.Uint16(frame[udpOffset+4 : udpOffset+6]))
	if udpLen < 8 || udpOffset+udpLen > l2Len+ipLen {
		return nil, 0, errInvalidFrame
	}

	request, err := dhcpv4.FromBytes(frame[udpOffset+8 : udpOffset+udpLen])
	if err != nil {
		return nil, 0, fmt.Errorf("decode DHCP request: %w", err)
	}
	return request, l2Len, nil
}

// encodeDHCPReply serializes a DHCP message into an Ethernet/IPv4/UDP frame.
// The original layer-2 header length is used to retain any VLAN tags.
func encodeDHCPReply(
	request *dhcpv4.DHCPv4,
	messageType dhcpv4.MessageType,
	serverHW net.HardwareAddr,
	serverIP net.IP,
	yourIP net.IP,
	mask net.IPMask,
	l2Header []byte,
) ([]byte, error) {
	reply, err := dhcpv4.NewReplyFromRequest(request,
		dhcpv4.WithMessageType(messageType),
		dhcpv4.WithYourIP(yourIP),
		dhcpv4.WithServerIP(serverIP),
		dhcpv4.WithBroadcast(true),
		dhcpv4.WithNetmask(mask),
		dhcpv4.WithLeaseTime(dhcpLeaseTime),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(serverIP)),
		dhcpv4.WithOption(dhcpv4.OptRouter(serverIP)),
	)
	if err != nil {
		return nil, fmt.Errorf("create DHCP reply: %w", err)
	}
	payload := reply.ToBytes()
	l2Len := len(l2Header)
	if l2Len < 14 || l2Len > 14+4*4 {
		return nil, errInvalidFrame
	}

	frame := make([]byte, l2Len+20+8+len(payload))
	copy(frame, l2Header)
	for i := 0; i < 6; i++ {
		frame[i] = 0xff
	}
	copy(frame[6:12], serverHW)
	// Retain the original VLAN tags, if present, and set the encapsulated
	// protocol to IPv4.
	if l2Len == 14 {
		binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	} else {
		binary.BigEndian.PutUint16(frame[l2Len-2:l2Len], 0x0800)
	}

	ip := frame[l2Len : l2Len+20]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(20+8+len(payload)))
	// IP identification is not significant for this unfragmented response.
	ip[8] = 64
	ip[9] = 17
	copy(ip[12:16], serverIP.To4())
	copy(ip[16:20], net.IPv4bcast.To4())
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))

	udp := frame[l2Len+20 : l2Len+28]
	binary.BigEndian.PutUint16(udp[0:2], dhcpServerPort)
	binary.BigEndian.PutUint16(udp[2:4], dhcpClientPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(payload)))
	// A zero UDP checksum is valid for IPv4.
	copy(frame[l2Len+28:], payload)

	// Ethernet frames must be at least 60 bytes before the FCS.
	if len(frame) < 60 {
		frame = append(frame, make([]byte, 60-len(frame))...)
	}
	return frame, nil
}

// checksum returns the Internet checksum for an IPv4 header.
func checksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data[:2]))
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func uint32IP(value uint32) net.IP {
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, value)
	return ip
}

func networkIP(value uint32) []byte {
	ip := make([]byte, net.IPv4len)
	binary.BigEndian.PutUint32(ip, value)
	return ip
}
