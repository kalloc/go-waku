package node

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/waku-org/go-waku/waku/v2/utils"
	"go.uber.org/zap"
)

func (w *WakuNode) newLocalnode(priv *ecdsa.PrivateKey) (*enode.LocalNode, error) {
	db, err := enode.OpenDB("")
	if err != nil {
		return nil, err
	}
	return enode.NewLocalNode(db, priv), nil
}

func writeMultiaddressField(localnode *enode.LocalNode, addrAggr []ma.Multiaddr) (err error) {
	defer func() {
		if e := recover(); e != nil {
			// Deleting the multiaddr entry, as we could not write it succesfully
			localnode.Delete(enr.WithEntry(utils.MultiaddrENRField, struct{}{}))
			err = errors.New("could not write enr record")
		}
	}()

	var fieldRaw []byte
	for _, addr := range addrAggr {
		maRaw := addr.Bytes()
		maSize := make([]byte, 2)
		binary.BigEndian.PutUint16(maSize, uint16(len(maRaw)))

		fieldRaw = append(fieldRaw, maSize...)
		fieldRaw = append(fieldRaw, maRaw...)
	}

	if len(fieldRaw) != 0 && len(fieldRaw) <= 100 { // Max length for multiaddr field before triggering the 300 bytes limit
		localnode.Set(enr.WithEntry(utils.MultiaddrENRField, fieldRaw))
	}

	// This is to trigger the signing record err due to exceeding 300bytes limit
	_ = localnode.Node()

	return nil
}

func (w *WakuNode) updateLocalNode(localnode *enode.LocalNode, multiaddrs []ma.Multiaddr, ipAddr *net.TCPAddr, udpPort uint, wakuFlags utils.WakuEnrBitfield, advertiseAddr []ma.Multiaddr, shouldAutoUpdate bool, log *zap.Logger) error {
	localnode.SetFallbackUDP(int(udpPort))
	localnode.Set(enr.WithEntry(utils.WakuENRField, wakuFlags))
	localnode.SetFallbackIP(net.IP{127, 0, 0, 1})

	if udpPort > math.MaxUint16 {
		return errors.New("invalid udp port number")
	}

	if advertiseAddr != nil {
		// An advertised address disables libp2p address updates
		// and discv5 predictions

		ipAddr, err := selectMostExternalAddress(advertiseAddr)
		if err != nil {
			return err
		}

		localnode.SetStaticIP(ipAddr.IP)
		localnode.Set(enr.TCP(uint16(ipAddr.Port))) // TODO: ipv6?

		return writeMultiaddresses(localnode, multiaddrs)
	} else if !shouldAutoUpdate {
		// We received a libp2p address update. Autoupdate is disabled
		// Using a static ip will disable endpoint prediction.
		localnode.SetStaticIP(ipAddr.IP)
		localnode.Set(enr.TCP(uint16(ipAddr.Port))) // TODO: ipv6?
		return writeMultiaddresses(localnode, multiaddrs)
	} else {
		// We received a libp2p address update, but we should still
		// allow discv5 to update the enr record. We set the localnode
		// keys manually. It's possible that the ENR record might get
		// updated automatically
		ip4 := ipAddr.IP.To4()
		ip6 := ipAddr.IP.To16()
		if ip4 != nil && !ip4.IsUnspecified() {
			localnode.Set(enr.IPv4(ip4))
			localnode.Set(enr.TCP(uint16(ipAddr.Port)))
		} else {
			localnode.Delete(enr.IPv4{})
			localnode.Delete(enr.TCP(0))
		}

		if ip4 == nil && ip6 != nil && !ip6.IsUnspecified() {
			localnode.Set(enr.IPv6(ip6))
			localnode.Set(enr.TCP6(ipAddr.Port))
		} else {
			localnode.Delete(enr.IPv6{})
			localnode.Delete(enr.TCP6(0))
		}

		return writeMultiaddresses(localnode, multiaddrs)
	}

}

func writeMultiaddresses(localnode *enode.LocalNode, multiaddrs []ma.Multiaddr) error {
	// Randomly shuffle multiaddresses
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(multiaddrs), func(i, j int) { multiaddrs[i], multiaddrs[j] = multiaddrs[j], multiaddrs[i] })

	// Adding extra multiaddresses. Should probably not exceed the enr max size of 300bytes
	var err error
	failedOnceWritingENR := false
	couldWriteENRatLeastOnce := false
	successIdx := -1
	for i := len(multiaddrs) - 1; i >= 0; i-- {
		err = writeMultiaddressField(localnode, multiaddrs[0:i])
		if err == nil {
			couldWriteENRatLeastOnce = true
			successIdx = i
			break
		} else {
			failedOnceWritingENR = true
		}
	}

	if failedOnceWritingENR && couldWriteENRatLeastOnce {
		// Could write a subset of multiaddresses but not all
		err = writeMultiaddressField(localnode, multiaddrs[0:successIdx])
		if err != nil {
			return errors.New("could not write new ENR")
		}
	}

	return nil
}

func isPrivate(addr *net.TCPAddr) bool {
	return addr.IP.IsPrivate()
}

func isExternal(addr *net.TCPAddr) bool {
	return !isPrivate(addr) && !addr.IP.IsLoopback() && !addr.IP.IsUnspecified()
}

func isLoopback(addr *net.TCPAddr) bool {
	return addr.IP.IsLoopback()
}

func filterIP(ss []*net.TCPAddr, fn func(*net.TCPAddr) bool) (ret []*net.TCPAddr) {
	for _, s := range ss {
		if fn(s) {
			ret = append(ret, s)
		}
	}
	return
}

func extractIPAddressForENR(addr ma.Multiaddr) (*net.TCPAddr, error) {
	// It's a p2p-circuit address. We shouldnt use these
	// for building the ENR record default keys
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	if err == nil {
		return nil, errors.New("can't use IP address from a p2p-circuit address")
	}

	// ws and wss addresses are handled by the multiaddr key
	// they shouldnt be used for building the ENR record default keys
	_, err = addr.ValueForProtocol(ma.P_WS)
	if err == nil {
		return nil, errors.New("can't use IP address from a ws address")
	}
	_, err = addr.ValueForProtocol(ma.P_WSS)
	if err == nil {
		return nil, errors.New("can't use IP address from a wss address")
	}

	var ipStr string
	dns4, err := addr.ValueForProtocol(ma.P_DNS4)
	if err != nil {
		ipStr, err = addr.ValueForProtocol(ma.P_IP4)
		if err != nil {
			return nil, err
		}
	} else {
		netIP, err := net.ResolveIPAddr("ip4", dns4)
		if err != nil {
			return nil, err
		}
		ipStr = netIP.String()
	}

	portStr, err := addr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	return &net.TCPAddr{
		IP:   net.ParseIP(ipStr),
		Port: port,
	}, nil
}

func selectMostExternalAddress(addresses []ma.Multiaddr) (*net.TCPAddr, error) {
	var ipAddrs []*net.TCPAddr
	for _, addr := range addresses {
		ipAddr, err := extractIPAddressForENR(addr)
		if err != nil {
			continue
		}
		ipAddrs = append(ipAddrs, ipAddr)
	}

	externalIPs := filterIP(ipAddrs, isExternal)
	if len(externalIPs) > 0 {
		return externalIPs[0], nil
	}

	privateIPs := filterIP(ipAddrs, isPrivate)
	if len(privateIPs) > 0 {
		return privateIPs[0], nil
	}

	loopback := filterIP(ipAddrs, isLoopback)
	if len(loopback) > 0 {
		return loopback[0], nil
	}

	return nil, errors.New("could not obtain ip address")
}

func decapsulateP2P(addr ma.Multiaddr) (ma.Multiaddr, error) {
	p2p, err := addr.ValueForProtocol(ma.P_P2P)
	if err != nil {
		return nil, err
	}

	p2pAddr, err := ma.NewMultiaddr("/p2p/" + p2p)
	if err != nil {
		return nil, err
	}

	addr = addr.Decapsulate(p2pAddr)

	return addr, nil
}

func decapsulateCircuitRelayAddr(addr ma.Multiaddr) (ma.Multiaddr, error) {
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	if err != nil {
		return nil, errors.New("not a circuit relay address")
	}

	// We remove the node's multiaddress from the addr
	addr, _ = ma.SplitFunc(addr, func(c ma.Component) bool {
		return c.Protocol().Code == ma.P_CIRCUIT
	})

	return addr, nil
}

func selectWSSListenAddresses(addresses []ma.Multiaddr) ([]ma.Multiaddr, error) {
	var result []ma.Multiaddr
	for _, addr := range addresses {
		// It's a p2p-circuit address. We dont use these at this stage yet
		_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
		if err == nil {
			continue
		}

		// Only WSS with a domain name are allowed
		_, err = addr.ValueForProtocol(ma.P_DNS4)
		if err != nil {
			continue
		}

		_, err = addr.ValueForProtocol(ma.P_WSS)
		if err != nil {
			continue
		}

		addr, err = decapsulateP2P(addr)
		if err == nil {
			result = append(result, addr)
		}
	}

	return result, nil
}

func selectCircuitRelayListenAddresses(addresses []ma.Multiaddr) ([]ma.Multiaddr, error) {
	var result []ma.Multiaddr
	for _, addr := range addresses {
		addr, err := decapsulateCircuitRelayAddr(addr)
		if err != nil {
			continue
		}
		result = append(result, addr)
	}

	return result, nil
}

func (w *WakuNode) getENRAddresses(addrs []ma.Multiaddr) (extAddr *net.TCPAddr, multiaddr []ma.Multiaddr, err error) {

	extAddr, err = selectMostExternalAddress(addrs)
	if err != nil {
		return nil, nil, err
	}

	wssAddrs, err := selectWSSListenAddresses(addrs)
	if err != nil {
		return nil, nil, err
	}

	multiaddr = append(multiaddr, wssAddrs...)

	// to use WSS, you should have a valid certificate with a domain name.
	// that means you're reachable. So circuit relay addresses are ignored
	if len(wssAddrs) == 0 {
		circuitAddrs, err := selectCircuitRelayListenAddresses(addrs)
		if err != nil {
			return nil, nil, err
		}

		multiaddr = append(multiaddr, circuitAddrs...)
	}

	return
}

func (w *WakuNode) setupENR(ctx context.Context, addrs []ma.Multiaddr) error {
	ipAddr, multiaddresses, err := w.getENRAddresses(addrs)
	if err != nil {
		w.log.Error("obtaining external address", zap.Error(err))
		return err
	}

	err = w.updateLocalNode(w.localNode, multiaddresses, ipAddr, w.opts.udpPort, w.wakuFlag, w.opts.advertiseAddrs, w.opts.discV5autoUpdate, w.log)
	if err != nil {
		w.log.Error("updating localnode ENR record", zap.Error(err))
		return err
	}

	return nil
}
