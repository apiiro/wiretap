package mapping

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

type HostMapping struct {
	Host  string
	Ports []uint16
}

func SetupFromConfig(s *stack.Stack, sendToServer bool) {
	hostsMapping, err := parseHostsMapping(viper.GetString("Mapping.Hosts"))
	if err != nil {
		log.Fatalln("Error parsing hosts mapping", err)
	}

	for _, mapping := range hostsMapping {
		log.Printf("Host: %s, Ports: %v\n", mapping.Host, mapping.Ports)
	}

	mappingPrefix := viper.GetString("Mapping.Prefix") + "."

	if net.ParseIP(mappingPrefix+"0") == nil {
		log.Fatalln("Invalid mapping prefix", mappingPrefix)
	}

	if sendToServer {
		SendConfig(hostsMapping, mappingPrefix)
	}

	setup(s, mappingPrefix, hostsMapping)
}

// HOSTS: "a.com:80:443,b.com:123,10.4.1.2:80:8080:81,h.com,x.com:123"
func parseHostsMapping(input string) ([]HostMapping, error) {
	var result []HostMapping

	// iterate over host-port pairs
	for _, mapping := range strings.Split(input, ",") {

		// Remove leading or trailing spaces
		mapping = strings.TrimSpace(mapping)

		if len(mapping) == 0 {
			continue
		}

		parts := strings.Split(mapping, ":")

		host := parts[0]
		var ports []uint16

		if len(parts[1:]) > 0 {
			for _, port := range parts[1:] {
				// Parse string to uint
				portUint, err := strconv.ParseUint(port, 10, 16)
				if err != nil {
					return nil, fmt.Errorf("invalid port value '%s': %w", port, err)
				}
				ports = append(ports, uint16(portUint))
			}
		} else {
			// Use default ports if none are provided
			ports = []uint16{80, 443}
		}

		result = append(result, HostMapping{
			Host:  host,
			Ports: ports,
		})
	}

	return result, nil
}

func setup(s *stack.Stack, mappingPrefix string, hostMappings []HostMapping) {
	log.Println("Mapping IPs", mappingPrefix)
	setupNATMasquarade(
		s,
		ipv4.ProtocolNumber,
		mappingPrefix,
		hostMappings,
	)
}

func setupNATMasquarade(s *stack.Stack, netProto tcpip.NetworkProtocolNumber, mappingPrefix string, hostMappings []HostMapping) {

	ipv6 := netProto == ipv6.ProtocolNumber
	ipt := s.IPTables()

	dstMask := net.ParseIP("255.255.255.255").To4()
	rules := make([]stack.Rule, 0)

	for i, mapping := range hostMappings {
		mappedIp, err := resolveIP(mapping.Host)

		if err != nil {
			continue
		}

		for _, port := range mapping.Ports {
			rule := stack.Rule{
				Filter: stack.IPHeaderFilter{
					CheckProtocol: false,
					Dst:           tcpip.AddrFrom4Slice(net.ParseIP(mappingPrefix + strconv.Itoa(i+1)).To4()),
					DstMask:       tcpip.AddrFrom4Slice(dstMask),
				},
				Target: &stack.DNATTarget{
					NetworkProtocol: ipv4.ProtocolNumber,
					Addr:            tcpip.AddrFrom4Slice(mappedIp.To4()),
					Port:            port,
				},
				Matchers: []stack.Matcher{
					&TCPMatcher{
						destinationPort: port,
					},
				},
			}

			rules = append(rules, rule)
		}
	}

	preroutes := len(rules)

	// Add prerouting drop
	rules = append(rules, stack.Rule{Target: &stack.DropTarget{}})

	// Add Input accept
	rules = append(rules, stack.Rule{Target: &stack.AcceptTarget{}})

	// Add Forward accept
	rules = append(rules, stack.Rule{Target: &stack.AcceptTarget{}})

	// Add Output accept
	rules = append(rules, stack.Rule{Target: &stack.AcceptTarget{}})

	// Add Postrouting (masquarde)
	rules = append(rules, stack.Rule{
		Filter: stack.IPHeaderFilter{
			Protocol:      tcp.ProtocolNumber,
			CheckProtocol: false,
		},
		Target: &stack.MasqueradeTarget{NetworkProtocol: netProto},
	})

	// Add Postrouting accept
	rules = append(rules, stack.Rule{Target: &stack.AcceptTarget{}})

	table := stack.Table{
		Rules: rules,
		BuiltinChains: [stack.NumHooks]int{
			stack.Prerouting:  0,
			stack.Input:       preroutes + 1,
			stack.Forward:     preroutes + 2,
			stack.Output:      preroutes + 3,
			stack.Postrouting: preroutes + 4,
		},
	}

	ipt.ReplaceTable(stack.NATID, table, ipv6)
}

func resolveIP(host string) (net.IP, error) {
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname is not in IP format, resolve it
		resolvedIP, err := net.ResolveIPAddr("ip4", host)
		if err != nil {
			log.Println("Unable to resolve IP", host, err.Error())
			return nil, err
		} else {
			if viper.GetBool("verbose") {
				log.Println("Resolved IP", host, resolvedIP.IP)
			}
			return resolvedIP.IP, nil
		}
	} else {
		// Hostname is already an IP address
		return ip, nil
	}
}

type TCPMatcher struct {
	destinationPort uint16
}

func (tm *TCPMatcher) Match(hook stack.Hook, pkt stack.PacketBufferPtr, _, _ string) (bool, bool) {
	switch pkt.NetworkProtocolNumber {
	case header.IPv4ProtocolNumber:
		netHeader := header.IPv4(pkt.NetworkHeader().Slice())
		if netHeader.TransportProtocol() != header.TCPProtocolNumber {
			return false, false
		}

		// We don't match fragments.
		if frag := netHeader.FragmentOffset(); frag != 0 {
			if frag == 1 {
				return false, true
			}
			return false, false
		}

	case header.IPv6ProtocolNumber:
		// As in Linux, we do not perform an IPv6 fragment check. See
		// xt_action_param.fragoff in
		// include/linux/netfilter/x_tables.h.
		if header.IPv6(pkt.NetworkHeader().Slice()).TransportProtocol() != header.TCPProtocolNumber {
			return false, false
		}

	default:
		// We don't know the network protocol.
		return false, false
	}

	tcpHeader := header.TCP(pkt.TransportHeader().Slice())
	if len(tcpHeader) < header.TCPMinimumSize {
		// There's no valid TCP header here, so we drop the packet immediately.
		return false, true
	}

	// Check whether the source and destination ports are within the
	// matching range.
	if destinationPort := tcpHeader.DestinationPort(); destinationPort != tm.destinationPort {
		return false, false
	}

	return true, false
}
