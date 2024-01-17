package cmd

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	gtcp "gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	gudp "gvisor.dev/gvisor/pkg/tcpip/transport/udp"

	"wiretap/peer"
	"wiretap/transport/icmp"
	"wiretap/transport/mapping"
	"wiretap/transport/tcp"
	"wiretap/transport/udp"
	"wiretap/transport/userspace"
)

type serveCmdConfig struct {
	configFile        string
	clientAddr4E2EE   string
	clientAddr6E2EE   string
	clientAddr4Relay  string
	clientAddr6Relay  string
	quiet             bool
	verbose           bool
	simple            bool
	logging           bool
	logFile           string
	catchTimeout      uint
	connTimeout       uint
	keepaliveIdle     uint
	keepaliveCount    uint
	keepaliveInterval uint
	disableV6         bool
}

type wiretapDefaultConfig struct {
	endpointRelay    string
	endpointE2EE     string
	port             int
	allowedIPs       string
	serverAddr4Relay string
	serverAddr6Relay string
	serverAddr4E2EE  string
	serverAddr6E2EE  string
	apiAddr          string
	apiV4Addr        string
	keepalive        int
	mtu              int
}

// Defaults for serve command.
var serveCmd = serveCmdConfig{
	configFile:        "",
	clientAddr4E2EE:   ClientE2EESubnet4.Addr().Next().String(),
	clientAddr6E2EE:   ClientE2EESubnet6.Addr().Next().String(),
	clientAddr4Relay:  ClientRelaySubnet4.Addr().Next().Next().String(),
	clientAddr6Relay:  ClientRelaySubnet6.Addr().Next().Next().String(),
	quiet:             false,
	verbose:           false,
	simple:            true,
	logging:           false,
	logFile:           "wiretap.log",
	catchTimeout:      5 * 1000,
	connTimeout:       5 * 1000,
	keepaliveIdle:     60,
	keepaliveCount:    3,
	keepaliveInterval: 60,
	disableV6:         false,
}

var wiretapDefault = wiretapDefaultConfig{
	endpointRelay:    Endpoint,
	endpointE2EE:     Endpoint,
	port:             Port,
	allowedIPs:       fmt.Sprintf("%s,%s", ClientRelaySubnet4.Addr().Next().String()+"/32", ClientRelaySubnet6.Addr().Next().String()+"/128"),
	serverAddr4Relay: RelaySubnets4.Addr().Next().Next().String(),
	serverAddr6Relay: RelaySubnets6.Addr().Next().Next().String(),
	serverAddr4E2EE:  E2EESubnets4.Addr().Next().Next().String(),
	serverAddr6E2EE:  E2EESubnets6.Addr().Next().Next().String(),
	apiAddr:          ApiSubnets.Addr().Next().Next().String(),
	apiV4Addr:        ApiV4Subnets.Addr().Next().Next().String(),
	keepalive:        Keepalive,
	mtu:              MTU,
}

// Add serve command and set flags.
func init() {
	var err error

	// Usage info.
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Listen and proxy traffic into target network",
		Long:  `Listen and proxy traffic into target network`,
		Run: func(cmd *cobra.Command, args []string) {
			serveCmd.Run()
		},
	}

	rootCmd.AddCommand(cmd)

	viper.Set("simple", true)

	// Flags.
	cmd.Flags().StringVarP(&serveCmd.configFile, "config-file", "f", serveCmd.configFile, "wireguard config file to read from")
	cmd.Flags().BoolVarP(&serveCmd.quiet, "quiet", "q", serveCmd.quiet, "silence wiretap log messages")
	cmd.Flags().BoolVarP(&serveCmd.verbose, "verbose", "v", serveCmd.verbose, "enable verbose log messages")
	cmd.Flags().BoolVarP(&serveCmd.simple, "simple", "", serveCmd.simple, "disable multihop and multiclient features for a simpler setup")
	cmd.Flags().BoolVarP(&serveCmd.logging, "log", "l", serveCmd.logging, "enable logging to file")
	cmd.Flags().StringVarP(&serveCmd.logFile, "log-file", "o", serveCmd.logFile, "write log to this filename")
	cmd.Flags().UintVarP(&serveCmd.catchTimeout, "completion-timeout", "", serveCmd.catchTimeout, "time in ms for client to complete TCP connection to server")
	cmd.Flags().UintVarP(&serveCmd.connTimeout, "conn-timeout", "", serveCmd.connTimeout, "time in ms for server to wait for outgoing TCP handshakes to complete")
	cmd.Flags().UintVarP(&serveCmd.keepaliveIdle, "keepalive-idle", "", serveCmd.keepaliveIdle, "time in seconds before TCP keepalives are sent to client")
	cmd.Flags().UintVarP(&serveCmd.keepaliveInterval, "keepalive-interval", "", serveCmd.keepaliveInterval, "time in seconds between TCP keepalives")
	cmd.Flags().UintVarP(&serveCmd.keepaliveCount, "keepalive-count", "", serveCmd.keepaliveCount, "number of unacknowledged TCP keepalives before closing connection")
	cmd.Flags().BoolVarP(&serveCmd.disableV6, "disable-ipv6", "", serveCmd.disableV6, "disable ipv6")

	cmd.Flags().StringVarP(&serveCmd.clientAddr4Relay, "ipv4-relay-client", "", serveCmd.clientAddr4Relay, "ipv4 relay address of client")
	cmd.Flags().StringVarP(&serveCmd.clientAddr6Relay, "ipv6-relay-client", "", serveCmd.clientAddr6Relay, "ipv6 relay address of client")
	cmd.Flags().StringVarP(&serveCmd.clientAddr4E2EE, "ipv4-e2ee-client", "", serveCmd.clientAddr4E2EE, "ipv4 e2ee address of client")
	cmd.Flags().StringVarP(&serveCmd.clientAddr6E2EE, "ipv6-e2ee-client", "", serveCmd.clientAddr6E2EE, "ipv6 e2ee address of client")

	// Bind supported flags to environment variables.
	err = viper.BindPFlag("simple", cmd.Flags().Lookup("simple"))
	check("error binding flag to viper", err)

	err = viper.BindPFlag("disableipv6", cmd.Flags().Lookup("disable-ipv6"))
	check("error binding flag to viper", err)

	err = viper.BindPFlag("verbose", cmd.Flags().Lookup("verbose"))
	check("error binding flag to viper", err)

	// Quiet and verbose flags must be used independently.
	cmd.MarkFlagsMutuallyExclusive("verbose", "quiet")

	// Deprecated flags, kept for backwards compatibility.
	cmd.Flags().StringP("private-relay", "", "", "wireguard private key for relay interface")
	cmd.Flags().StringP("public-relay", "", "", "wireguard public key of remote peer for relay interface")
	cmd.Flags().StringP("private-e2ee", "", "", "wireguard private key for E2EE interface")
	cmd.Flags().StringP("public-e2ee", "", "", "wireguard public key of remote peer for E2EE interface")
	cmd.Flags().StringP("endpoint-relay", "", wiretapDefault.endpointRelay, "socket address of remote peer that server will connect to (example \"1.2.3.4:51820\")")
	cmd.Flags().StringP("endpoint-e2ee", "", wiretapDefault.endpointE2EE, "socket address of remote peer's e2ee interface that server will connect to (example \"1.2.3.4:51820\")")
	cmd.Flags().IntP("port", "p", wiretapDefault.port, "wireguard listener port")
	cmd.Flags().StringP("allowed", "a", wiretapDefault.allowedIPs, "comma-separated list of CIDR IP ranges to associate with peer")
	cmd.Flags().StringP("ipv4-relay", "", wiretapDefault.serverAddr4Relay, "ipv4 relay address")
	cmd.Flags().StringP("ipv6-relay", "", wiretapDefault.serverAddr6Relay, "ipv6 relay address")
	cmd.Flags().StringP("ipv4-e2ee", "", wiretapDefault.serverAddr4E2EE, "ipv4 e2ee address")
	cmd.Flags().StringP("ipv6-e2ee", "", wiretapDefault.serverAddr6E2EE, "ipv6 e2ee address")
	cmd.Flags().StringP("api", "0", wiretapDefault.apiAddr, "address of API service")
	cmd.Flags().IntP("keepalive", "k", wiretapDefault.keepalive, "tunnel keepalive in seconds")
	cmd.Flags().IntP("mtu", "m", wiretapDefault.mtu, "tunnel MTU")

	// Bind deprecated flags to viper.
	err = viper.BindPFlag("Relay.Interface.privatekey", cmd.Flags().Lookup("private-relay"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Interface.port", cmd.Flags().Lookup("port"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Interface.ipv4", cmd.Flags().Lookup("ipv4-relay"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Interface.ipv6", cmd.Flags().Lookup("ipv6-relay"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Interface.mtu", cmd.Flags().Lookup("mtu"))
	check("error binding flag to viper", err)

	err = viper.BindPFlag("Relay.Peer.publickey", cmd.Flags().Lookup("public-relay"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Peer.endpoint", cmd.Flags().Lookup("endpoint-relay"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Peer.allowed", cmd.Flags().Lookup("allowed"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("Relay.Peer.keepalive", cmd.Flags().Lookup("keepalive"))
	check("error binding flag to viper", err)

	err = viper.BindPFlag("E2EE.Interface.privatekey", cmd.Flags().Lookup("private-e2ee"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("E2EE.Interface.ipv4", cmd.Flags().Lookup("ipv4-e2ee"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("E2EE.Interface.ipv6", cmd.Flags().Lookup("ipv6-e2ee"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("E2EE.Interface.api", cmd.Flags().Lookup("api"))
	check("error binding flag to viper", err)

	err = viper.BindPFlag("E2EE.Peer.publickey", cmd.Flags().Lookup("public-e2ee"))
	check("error binding flag to viper", err)
	err = viper.BindPFlag("E2EE.Peer.endpoint", cmd.Flags().Lookup("endpoint-e2ee"))
	check("error binding flag to viper", err)

	// Set default values for viper.
	viper.SetDefault("Relay.Interface.port", wiretapDefault.port)
	viper.SetDefault("Relay.Interface.ipv4", wiretapDefault.serverAddr4Relay)
	viper.SetDefault("Relay.Interface.ipv6", wiretapDefault.serverAddr6Relay)
	viper.SetDefault("Relay.Interface.mtu", wiretapDefault.mtu)

	viper.SetDefault("Relay.Interface.Peer.endpoint", wiretapDefault.endpointRelay)
	viper.SetDefault("Relay.Interface.Peer.allowed", wiretapDefault.allowedIPs)
	viper.SetDefault("Relay.Interface.Peer.keepalive", wiretapDefault.keepalive)

	viper.SetDefault("E2EE.Interface.ipv4", wiretapDefault.serverAddr4E2EE)
	viper.SetDefault("E2EE.Interface.ipv6", wiretapDefault.serverAddr6E2EE)
	viper.SetDefault("E2EE.Interface.api", wiretapDefault.apiAddr)

	viper.SetDefault("E2EE.Peer.endpoint", wiretapDefault.endpointE2EE)

	cmd.Flags().SortFlags = false

	// Hide deprecated flags and log flags.
	helpFunc := cmd.HelpFunc()
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !ShowHidden {
			for _, f := range []string{
				"ipv4-relay-client",
				"ipv6-relay-client",
				"ipv4-e2ee-client",
				"ipv6-e2ee-client",
				"private-relay",
				"public-relay",
				"private-e2ee",
				"public-e2ee",
				"endpoint-relay",
				"endpoint-e2ee",
				"port",
				"allowed",
				"ipv4-relay",
				"ipv6-relay",
				"ipv4-e2ee",
				"ipv6-e2ee",
				"api",
				"keepalive",
				"mtu",
				"conn-timeout",
				"completion-timeout",
				"keepalive-interval",
				"keepalive-count",
				"keepalive-idle",
				"disable-ipv6",
			} {
				err := cmd.Flags().MarkHidden(f)
				if err != nil {
					fmt.Printf("Failed to hide flag %v: %v\n", f, err)
				}
			}
		}
		helpFunc(cmd, args)
	})
}

// Run parses/processes/validates args and then connects to peer,
// proxying traffic from peer into local network.
func (c serveCmdConfig) Run() {
	// Read config from file and/or environment.
	viper.AutomaticEnv()
	viper.SetEnvPrefix("WIRETAP")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if c.configFile != "" {
		viper.SetConfigType("ini")
		viper.SetConfigFile(c.configFile)
		if err := viper.ReadInConfig(); err != nil {
			check("error reading config file", err)
		}
	}

	// Synchronization vars.
	var (
		wg   sync.WaitGroup
		lock sync.Mutex
	)

	// Configure logging.
	log.SetOutput(os.Stdout)
	log.SetPrefix("WIRETAP: ")
	if c.quiet {
		log.SetOutput(io.Discard)
	}
	if c.logging {
		f, err := os.OpenFile(c.logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		check("error opening log file", err)
		defer f.Close()

		if c.quiet {
			log.SetOutput(f)
		} else {
			log.SetOutput(io.MultiWriter(os.Stdout, f))
		}
	}

	// Check for required flags.
	if !viper.IsSet("Relay.Peer.publickey") || (!viper.IsSet("simple") && !viper.IsSet("E2EE.Peer.publickey")) {
		check("config error", errors.New("public key of peer is required"))
	}

	if viper.IsSet("disableipv6") && netip.MustParseAddr(viper.GetString("E2EE.Interface.api")).Is6() {
		viper.Set("E2EE.Interface.api", wiretapDefault.apiV4Addr)
	}

	relayAddresses := []string{viper.GetString("Relay.Interface.ipv4") + "/32"}
	if !viper.IsSet("disableipv6") {
		relayAddresses = append(relayAddresses, viper.GetString("Relay.Interface.ipv6")+"/128")
	}
	aips := []string{}
	for _, ip := range strings.Split(viper.GetString("Relay.Peer.allowed"), ",") {
		if viper.IsSet("disableipv6") && netip.MustParsePrefix(ip).Addr().Is6() {
			continue
		}

		aips = append(aips, ip)
	}
	configRelayArgs := peer.ConfigArgs{
		PrivateKey: viper.GetString("Relay.Interface.privatekey"),
		ListenPort: viper.GetInt("Relay.Interface.port"),
		Peers: []peer.PeerConfigArgs{
			{
				PublicKey: viper.GetString("Relay.Peer.publickey"),
				Endpoint:  viper.GetString("Relay.Peer.endpoint"),
				PersistentKeepaliveInterval: func() int {
					if len(viper.GetString("Relay.Peer.endpoint")) > 0 {
						return viper.GetInt("Relay.Peer.keepalive")
					} else {
						return 0
					}
				}(),
				AllowedIPs: aips,
			},
		},
		Addresses: relayAddresses,
	}

	configRelay, err := peer.GetConfig(configRelayArgs)
	check("failed to make relay configuration", err)

	allowedIPs := []string{c.clientAddr4E2EE + "/32"}
	if !viper.IsSet("disableipv6") {
		allowedIPs = append(allowedIPs, c.clientAddr6E2EE+"/128")
	}
	e2eeAddresses := []string{viper.GetString("E2EE.Interface.ipv4") + "/32"}
	if !viper.IsSet("disableipv6") {
		e2eeAddresses = append(e2eeAddresses, viper.GetString("E2EE.Interface.ipv6")+"/128")
	}
	var configE2EE peer.Config
	if !viper.GetBool("simple") {
		configE2EEArgs := peer.ConfigArgs{
			PrivateKey: viper.GetString("E2EE.Interface.privatekey"),
			ListenPort: E2EEPort,
			Peers: []peer.PeerConfigArgs{
				{
					PublicKey:                   viper.GetString("E2EE.Peer.publickey"),
					Endpoint:                    viper.GetString("E2EE.Peer.endpoint"),
					AllowedIPs:                  allowedIPs,
					PersistentKeepaliveInterval: viper.GetInt("Relay.Peer.keepalive"),
				},
			},
			Addresses: e2eeAddresses,
		}
		configE2EE, err = peer.GetConfig(configE2EEArgs)
		check("failed to make e2ee configuration", err)
	}

	// Print public key for easier configuration.
	fmt.Println()
	fmt.Println("Relay configuration:")
	fmt.Println(strings.Repeat("─", 32))
	fmt.Print(configRelay.AsShareableFile())
	fmt.Print()
	fmt.Println(strings.Repeat("─", 32))
	if !viper.GetBool("simple") {
		fmt.Println()
		fmt.Println("E2EE configuration:")
		fmt.Println(strings.Repeat("─", 32))
		fmt.Print(configE2EE.AsShareableFile())
		fmt.Println(strings.Repeat("─", 32))
	}
	fmt.Println()

	apiAddr, err := netip.ParseAddr(viper.GetString("E2EE.Interface.api"))
	check("failed to parse API address", err)

	// Create virtual relay interface with this address and MTU.
	ipv4Addr, err := netip.ParseAddr(viper.GetString("Relay.Interface.ipv4"))
	check("failed to parse ipv4 address", err)

	relayAddrs := []netip.Addr{ipv4Addr}

	if !viper.IsSet("disableipv6") {
		ipv6Addr, err := netip.ParseAddr(viper.GetString("Relay.Interface.ipv6"))
		check("failed to parse ipv6 address", err)
		relayAddrs = append(relayAddrs, ipv6Addr)
	}

	if viper.GetBool("simple") {
		relayAddrs = append(relayAddrs, apiAddr)
	}

	mtu := viper.GetInt("Relay.Interface.mtu")
	log.Printf("MTU: %d\n", mtu)
	tunRelay, tnetRelay, err := netstack.CreateNetTUN(
		relayAddrs,
		[]netip.Addr{},
		mtu,
	)
	check("failed to create relay TUN", err)

	var tunE2EE tun.Device
	var tnetE2EE *netstack.Net
	if !viper.GetBool("simple") {
		// Enable forwarding for Relay NICs
		s := tnetRelay.Stack()
		tcpipErr := s.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, true)
		if tcpipErr != nil {
			check("failed to enable forwarding", errors.New(tcpipErr.String()))
		}
		if !viper.IsSet("disableipv6") {
			tcpipErr = s.SetForwardingDefaultAndAllNICs(ipv6.ProtocolNumber, true)
			if tcpipErr != nil {
				check("failed to enable forwarding", errors.New(tcpipErr.String()))
			}
		}

		// Create virtual e2ee interface with this address and MTU - 80.
		ipv4Addr, err = netip.ParseAddr(viper.GetString("E2EE.Interface.ipv4"))
		check("failed to parse ipv4 address", err)

		e2eeAddrs := []netip.Addr{ipv4Addr, apiAddr}

		if !viper.IsSet("disableipv6") {
			ipv6Addr, err := netip.ParseAddr(viper.GetString("E2EE.Interface.ipv6"))
			check("failed to parse ipv6 address", err)
			e2eeAddrs = append(e2eeAddrs, ipv6Addr)
		}

		if !viper.GetBool("simple") {
			tunE2EE, tnetE2EE, err = netstack.CreateNetTUN(
				e2eeAddrs,
				[]netip.Addr{},
				viper.GetInt("Relay.Interface.mtu")-80,
			)
			check("failed to create E2EE TUN", err)
		}
	}

	transportHandler := func() *netstack.Net {
		if viper.GetBool("simple") {
			return tnetRelay
		} else {
			return tnetE2EE
		}
	}()

	var logger int
	if viper.GetBool("verbose") {
		logger = device.LogLevelVerbose
	} else if c.quiet {
		logger = device.LogLevelSilent
	} else {
		logger = device.LogLevelError
	}

	s := transportHandler.Stack()
	s.SetPromiscuousMode(1, true)

	// TCP Forwarding mechanism.
	tcpConfig := tcp.Config{
		CatchTimeout:      time.Duration(c.catchTimeout) * time.Millisecond,
		ConnTimeout:       time.Duration(c.connTimeout) * time.Millisecond,
		KeepaliveIdle:     time.Duration(c.keepaliveIdle) * time.Second,
		KeepaliveInterval: time.Duration(c.keepaliveInterval) * time.Second,
		KeepaliveCount:    int(c.keepaliveCount),
		Tnet:              transportHandler,
		StackLock:         &lock,
	}
	tcpForwarder := gtcp.NewForwarder(s, 0, 65535, tcp.Handler(tcpConfig))
	s.SetTransportProtocolHandler(gtcp.ProtocolNumber, tcpForwarder.HandlePacket)

	// UDP Forwarding mechanism.
	udpConfig := udp.Config{
		Tnet:      transportHandler,
		StackLock: &lock,
	}

	s.SetTransportProtocolHandler(gudp.ProtocolNumber, udp.Handler(udpConfig))

	// Make new relay device.
	devRelay := device.NewDevice(tunRelay, conn.NewDefaultBind(), device.NewLogger(logger, ""))
	// Configure wireguard.
	err = devRelay.IpcSet(configRelay.AsIPC())
	check("failed to configure relay wireguard device", err)
	err = devRelay.Up()
	check("failed to bring up relay device", err)

	var devE2EE *device.Device
	if !viper.GetBool("simple") {
		// Make new e2ee device, bind to relay device's userspace networking stack.
		devE2EE = device.NewDevice(tunE2EE, userspace.NewBind(tnetRelay), device.NewLogger(logger, ""))

		// Configure wireguard.
		fmt.Println(configE2EE.AsIPC())
		err = devE2EE.IpcSet(configE2EE.AsIPC())
		check("failed to configure e2ee wireguard device", err)
		err = devE2EE.Up()
		check("failed to bring up e2ee device", err)
	}

	// Handlers that require long-running routines:

	// IP mapping now, and every 10 minutes
	mapping.SetupFromConfig(s, true)
	mappingTicker := time.NewTicker(10 * time.Minute)
	wg.Add(1)
	go func() {
		for range mappingTicker.C {
			mapping.SetupFromConfig(s, false)
		}
		wg.Done()
	}()

	// Start ICMP Handler.
	wg.Add(1)
	go func() {
		icmp.Handle(transportHandler, &lock)
		wg.Done()
	}()

	// Start API handler.
	// wg.Add(1)
	// go func() {
	// 	ns := api.NetworkState{
	// 		NextClientRelayAddr4: netip.MustParseAddr(c.clientAddr4Relay),
	// 		NextClientRelayAddr6: netip.MustParseAddr(c.clientAddr6Relay),
	// 		NextServerRelayAddr4: netip.MustParseAddr(viper.GetString("Relay.Interface.ipv4")),
	// 		NextServerRelayAddr6: netip.MustParseAddr(viper.GetString("Relay.Interface.ipv6")),
	// 		NextClientE2EEAddr4:  netip.MustParseAddr(c.clientAddr4E2EE),
	// 		NextClientE2EEAddr6:  netip.MustParseAddr(c.clientAddr6E2EE),
	// 		NextServerE2EEAddr4:  netip.MustParseAddr(viper.GetString("E2EE.Interface.ipv4")),
	// 		NextServerE2EEAddr6:  netip.MustParseAddr(viper.GetString("E2EE.Interface.ipv6")),
	// 		ApiAddr:              netip.MustParseAddr(viper.GetString("E2EE.Interface.api")),
	// 	}
	// 	api.Handle(transportHandler, devRelay, devE2EE, &configRelay, &configE2EE, apiAddr, uint16(ApiPort), &lock, &ns)
	// 	wg.Done()
	// }()

	// Start Healthcheck handler
	wg.Add(1)
	go func() {
		http.HandleFunc("/health", handleHealth(devRelay))
		log.Fatal(http.ListenAndServe(":8080", nil))
		wg.Done()
	}()

	wg.Wait()
}

func handleHealth(devRelay *device.Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ipc, err := devRelay.IpcGet()
		if err != nil {
			writeErr(w, err)
			return
		}

		last_handshake_time_sec := int64(0)
		regex := regexp.MustCompile(`last_handshake_time_sec=(\d+)`) // Match "xcv=" followed by one or more digits
		match := regex.FindStringSubmatch(ipc)
		if len(match) > 1 {
			last_handshake_time_sec, _ = strconv.ParseInt(match[1], 10, 64)
		}

		if last_handshake_time_sec == 0 {
			w.WriteHeader(503)
			_, err = io.WriteString(w, "-1")
			if err != nil {
				writeErr(w, err)
			}
			return
		}

		secs_since_handshake := time.Now().Unix() - last_handshake_time_sec
		if secs_since_handshake > 180 {
			w.WriteHeader(503)
		}

		_, err = io.WriteString(w, "Seconds since handshake: "+strconv.Itoa(int(secs_since_handshake)))
		if err != nil {
			writeErr(w, err)
			return
		}
	}
}

func writeErr(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	_, err = io.WriteString(w, err.Error())
	if err != nil {
		log.Printf("API Error: %v", err)
	}
}
