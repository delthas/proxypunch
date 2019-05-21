package punch

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/delthas/proxypunch/mocknet"
)

const defaultPort = 41254

const flushInterval = 100 * time.Second // FIXME

var _, localIpv4, _ = net.ParseCIDR("127.0.0.0/8")
var _, localIpv6, _ = net.ParseCIDR("fc00::/7")

var soku = true

type addrKey struct {
	ip   [4]byte
	port int
}

func (s addrKey) toUDP() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   s.ip[:],
		Port: s.port,
	}
}

func newAddrKey(udp *net.UDPAddr) addrKey {
	var ip [4]byte
	copy(ip[:], udp.IP.To4())
	return addrKey{
		ip:   ip,
		port: udp.Port,
	}
}

type addrValue struct {
	c    mocknet.UDPConn
	last time.Time
}

func Client(mnet *mocknet.MockNet, relayHost string, host string, port int) {
	c, err := mnet.ListenUDP("udp4", &net.UDPAddr{
		Port: defaultPort,
	})
	if err != nil {
		c, err = mnet.ListenUDP("udp4", nil)
		if err != nil {
			log.Fatal(err)
		}
	}
	defer c.Close()

	localPort := c.LocalAddr().(*net.UDPAddr).Port
	fmt.Println("Listening, connect to 127.0.0.1:" + strconv.Itoa(localPort))

	relayAddr, err := net.ResolveUDPAddr("udp4", relayHost)
	if err != nil {
		log.Fatal(err)
	}

	serverAddr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(host, strconv.Itoa(0)))
	if err != nil {
		log.Fatal(err)
	}
	serverAddr.IP = serverAddr.IP.To4()

	mappings := make(map[addrKey]time.Time)
	mappingsMutex := sync.Mutex{}

	// relay & mappings keepalive
	chRelay := make(chan struct{})
	go func() {
		relayPayload := append([]byte{byte(port >> 8), byte(port)}, serverAddr.IP.To4()...)
		punchPayload := []byte{0xCD}
		flushTime := time.Now()
		for {
			select {
			case <-chRelay:
				return
			default:
			}

			// timeout old mappings
			now := time.Now()
			if now.Sub(flushTime) > flushInterval {
				flushTime = now
				mappingsMutex.Lock()
				for k, v := range mappings {
					if !v.IsZero() && now.Sub(v) > flushInterval {
						delete(mappings, k)
					}
				}
				mappingsMutex.Unlock()
			}

			c.WriteToUDP(relayPayload, relayAddr)
			mappingsMutex.Lock()
			for k := range mappings {
				c.WriteToUDP(punchPayload, k.toUDP())
			}
			mappingsMutex.Unlock()
			d := 500*time.Millisecond - time.Now().Sub(now)
			if d > 0 {
				time.Sleep(d)
			}
		}
	}()
	defer close(chRelay)

	buffer := make([]byte, 4096)

	for {
		n, addr, err := c.ReadFromUDP(buffer)
		if err != nil {
			// err is thrown if the buffer is too small
			continue
		}
		if !addr.IP.Equal(relayAddr.IP) || addr.Port != relayAddr.Port {
			continue
		}
		if n != 2 {
			fmt.Fprintln(os.Stderr, "Error received packet of wrong size from relay. (size:"+strconv.Itoa(n)+")")
			continue
		}
		serverAddr.Port = int(binary.BigEndian.Uint16(buffer[:2]))
		break
	}

	mappingsMutex.Lock()
	mappings[newAddrKey(serverAddr)] = time.Time{}
	mappingsMutex.Unlock()

	peers := make(map[addrKey]*addrValue)
	localMappings := make(map[int]addrKey)
	redirectCache := make(map[addrKey]time.Time)

	var remoteAddr *net.UDPAddr = nil
	var cRemote mocknet.UDPConn
	createPeer := func(addr *net.UDPAddr, server bool) mocknet.UDPConn {
		addrKey := newAddrKey(addr)

		cLocal, err := mnet.ListenUDP("udp4", nil)
		if err != nil {
			log.Fatal(err)
		}

		if server {
			mappingsMutex.Lock()
			if remoteAddr != nil {
				delete(mappings, newAddrKey(remoteAddr))
			}
			remoteAddr = addr
			mappings[newAddrKey(remoteAddr)] = time.Time{}
			mappingsMutex.Unlock()
			if cRemote != nil {
				delete(localMappings, cRemote.LocalAddr().(*net.UDPAddr).Port)
				cRemote.Close()
			}
			cRemote = cLocal
		}
		peers[addrKey] = &addrValue{
			c:    cLocal,
			last: time.Now(),
		}

		localMappings[cLocal.LocalAddr().(*net.UDPAddr).Port] = addrKey
		go func() { // receive from game client, forward to remote peer
			buffer := make([]byte, 4096)
			for {
				n, _, err := cLocal.ReadFromUDP(buffer[1:])
				if err != nil {
					if _, ok := peers[addrKey]; ok {
						fmt.Fprintln(os.Stderr, "Peer disconnected (read failed) with IP: "+addr.String())
						delete(peers, addrKey)
						delete(localMappings, cLocal.LocalAddr().(*net.UDPAddr).Port)
						cLocal.Close()
					}
					break
				}
				if n > len(buffer)-1 {
					fmt.Fprintln(os.Stderr, "Error received packet of wrong size from game client. (size:"+strconv.Itoa(n)+")")
					continue
				}

				if injectSoku(buffer, n, c, mnet, addr, addrKey, localMappings) {
					continue
				}

				buffer[0] = 0xCC
				c.WriteToUDP(buffer[:n+1], addr)
			}
		}()

		return cLocal
	}

	// connected to relay, main server<->peers loop
	flushTime := time.Now()
	foundPeer := false
	var localAddr net.UDPAddr
	for {
		// timeout old peers
		now := time.Now()
		if now.Sub(flushTime) > flushInterval {
			flushTime = now
			for k, v := range peers {
				if now.Sub(v.last) > flushInterval {
					fmt.Println("Peer disconnected (timeout) with IP: " + k.toUDP().IP.String())
					delete(peers, k)
					delete(localMappings, v.c.LocalAddr().(*net.UDPAddr).Port)
					v.c.Close()
				}
			}
		}

		n, addr, err := c.ReadFromUDP(buffer[1:])
		if err != nil {
			// err is thrown if the buffer is too small
			continue
		}
		if n > len(buffer)-1 {
			fmt.Fprintln(os.Stderr, "Error received packet of wrong size from peer. (size:"+strconv.Itoa(n)+")")
			continue
		}
		if addr.IP.Equal(relayAddr.IP) && addr.Port == relayAddr.Port {
			continue
		}
		addrKey := newAddrKey(addr)
		if (addr.IP.Equal(serverAddr.IP) && addr.Port == serverAddr.Port) ||
			(remoteAddr != nil && addr.IP.Equal(remoteAddr.IP) && addr.Port == remoteAddr.Port) {
			var cSend mocknet.UDPConn
			if addr.IP.Equal(serverAddr.IP) && addr.Port == serverAddr.Port {
				cSend = c
			} else {
				cSend = cRemote
			}
			if !foundPeer {
				foundPeer = true
				fmt.Println("Connected to server")
			}
			if n == 0 || localPort == 0 {
				continue
			}
			if buffer[1] == 0xCE && n >= 20 { // redirect to new server/client
				port := int(binary.BigEndian.Uint16(buffer[2:4]))
				ip := buffer[4:8]
				redirectAddr := &net.UDPAddr{
					IP:   net.IP(ip),
					Port: port,
				}
				redirectAddrKey := newAddrKey(redirectAddr)
				now := time.Now()
				skip := false
				for addrKey, last := range redirectCache {
					if now.Sub(last) > 2*time.Second {
						delete(redirectCache, addrKey)
					} else if addrKey == redirectAddrKey {
						skip = true
					}
				}
				if skip {
					continue
				}
				redirectCache[redirectAddrKey] = time.Now()
				fmt.Println("Redirecting to IP: " + net.IP(ip).String())
				cLocal := createPeer(redirectAddr, true)
				cLocalAddr := cLocal.LocalAddr().(*net.UDPAddr)
				cLocalAddr.IP = cLocalAddr.IP.To4()
				buffer[15] = byte(cLocalAddr.Port >> 8)
				buffer[16] = byte(cLocalAddr.Port)
				copy(buffer[17:21], cLocalAddr.IP)
				cSend.WriteToUDP(buffer[8:n+1], &localAddr)
			} else if buffer[1] == 0xCF && n >= 7 { // add new mapping
				port := int(binary.BigEndian.Uint16(buffer[2:4]))
				ip := buffer[4:8]
				mappingsMutex.Lock()
				mappings[newAddrKey(&net.UDPAddr{IP: net.IP(ip), Port: port})] = time.Now()
				mappingsMutex.Unlock()
			} else if buffer[1] == 0xCC {
				cSend.WriteToUDP(buffer[2:n+1], &localAddr)
			}
		} else if localIpv4.Contains(addr.IP) || localIpv6.Contains(addr.IP) { // game client
			localAddr = *addr
			buffer[0] = 0xCC
			c.WriteToUDP(buffer[:n+1], serverAddr)
		} else if v, ok := peers[addrKey]; ok { // existing peer
			if n != 0 && buffer[1] == 0xCC { // forward to game client
				v.c.WriteToUDP(buffer[2:n+1], &localAddr)
			}
			peers[addrKey].last = time.Now()
		} else { // new peer
			fmt.Println("New peer connected with IP: " + addr.IP.To4().String())
			cLocal := createPeer(addr, false)
			defer cLocal.Close()
		}
	}
}

func Server(mnet *mocknet.MockNet, relayHost string, port int) {
	c, err := mnet.ListenUDP("udp4", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	fmt.Println("Listening, start hosting on port " + strconv.Itoa(port))
	fmt.Println("Connecting...")

	localAddr := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: port,
	}

	relayAddr, err := net.ResolveUDPAddr("udp4", relayHost)
	if err != nil {
		log.Fatal(err)
	}

	mappings := make(map[addrKey]time.Time)
	mappingsMutex := sync.Mutex{}

	// relay & mappings keepalive
	chRelay := make(chan struct{})
	go func() {
		relayPayload := []byte{byte(port >> 8), byte(port)}
		punchPayload := []byte{0xCD}
		flushTime := time.Now()
		for {
			select {
			case <-chRelay:
				return
			default:
			}

			// timeout old mappings
			now := time.Now()
			if now.Sub(flushTime) > flushInterval {
				flushTime = now
				mappingsMutex.Lock()
				for k, v := range mappings {
					if now.Sub(v) > flushInterval {
						delete(mappings, k)
					}
				}
				mappingsMutex.Unlock()
			}

			c.WriteToUDP(relayPayload, relayAddr)
			mappingsMutex.Lock()
			for k := range mappings {
				c.WriteToUDP(punchPayload, k.toUDP())
			}
			mappingsMutex.Unlock()
			d := 500*time.Millisecond - time.Now().Sub(now)
			if d > 0 {
				time.Sleep(d)
			}
		}
	}()
	defer close(chRelay)

	buffer := make([]byte, 4096)

	// get external ip
	for {
		n, addr, err := c.ReadFromUDP(buffer)
		if err != nil {
			// err is thrown if the buffer is too small
			continue
		}
		if !addr.IP.Equal(relayAddr.IP) || addr.Port != relayAddr.Port {
			continue
		}
		if n < 4 || n%6 != 4 {
			fmt.Fprintln(os.Stderr, "Error received packet of wrong size from relay. (size:"+strconv.Itoa(n)+")")
			continue
		}
		for i, v := range buffer[:4] {
			buffer[i] = v ^ 0xCC // undo ip xor of relay
		}
		ip := net.IP(buffer[:4])
		fmt.Println("Connected. Ask your peers to connect with proxypunch to " + ip.String() + ":" + strconv.Itoa(port))
		break
	}

	// connected to relay, main server<->peers and relay loop
	peers := make(map[addrKey]*addrValue)
	localMappings := make(map[int]addrKey)
	flushTime := time.Now()
	for {
		// timeout old peers
		now := time.Now()
		if now.Sub(flushTime) > flushInterval {
			flushTime = now
			for k, v := range peers {
				if now.Sub(v.last) > flushInterval {
					fmt.Println("Peer disconnected (timeout) with IP: " + k.toUDP().IP.String())
					delete(peers, k)
					delete(localMappings, v.c.LocalAddr().(*net.UDPAddr).Port)
					v.c.Close()
				}
			}
		}

		n, addr, err := c.ReadFromUDP(buffer[1:])
		if err != nil {
			// err is thrown if the buffer is too small
			continue
		}
		if n > len(buffer)-1 {
			fmt.Fprintln(os.Stderr, "Error received packet of wrong size from peer. (size:"+strconv.Itoa(n)+")")
			continue
		}
		if addr.IP.Equal(relayAddr.IP) && addr.Port == relayAddr.Port {
			if n < 4 || n%6 != 4 {
				fmt.Fprintln(os.Stderr, "Error received packet of wrong size from relay. (size:"+strconv.Itoa(n)+")")
				continue
			}
			for i := 5; i < n+1; i += 6 {
				port := int(binary.BigEndian.Uint16(buffer[i : i+2]))
				var ip [4]byte
				copy(ip[:], buffer[i+2:i+6])
				mappingsMutex.Lock()
				mappings[addrKey{
					ip:   ip,
					port: port,
				}] = time.Now()
				mappingsMutex.Unlock()
			}
			continue
		}
		addrKey := newAddrKey(addr)
		if v, ok := peers[addrKey]; ok { // existing peer
			if n != 0 && buffer[1] == 0xCC { // forward to server
				v.c.WriteToUDP(buffer[2:n+1], localAddr)
			}
			peers[addrKey].last = time.Now()
		} else { // new peer
			fmt.Println("New peer connected with IP: " + addr.IP.To4().String())
			cLocal, err := mnet.ListenUDP("udp4", nil)
			if err != nil {
				log.Fatal(err)
			}
			peers[addrKey] = &addrValue{
				c:    cLocal,
				last: time.Now(),
			}
			localMappings[cLocal.LocalAddr().(*net.UDPAddr).Port] = addrKey
			go func() { // receive from server, forward to remote peer
				buffer := make([]byte, 4096)
				for {
					n, _, err := cLocal.ReadFromUDP(buffer[1:])
					if err != nil {
						if _, ok := peers[addrKey]; ok {
							fmt.Fprintln(os.Stderr, "Peer disconnected (read failed) with IP: "+addr.String())
							delete(peers, addrKey)
							delete(localMappings, cLocal.LocalAddr().(*net.UDPAddr).Port)
							cLocal.Close()
						}
						break
					}
					if n > len(buffer)-1 {
						fmt.Fprintln(os.Stderr, "Error received packet of wrong size from game server. (size:"+strconv.Itoa(n)+")")
						continue
					}

					if injectSoku(buffer, n, c, mnet, addr, addrKey, localMappings) {
						continue
					}

					buffer[0] = 0xCC
					c.WriteToUDP(buffer[:n+1], addr)
				}
			}()
			defer cLocal.Close()
		}
	}
}

func injectSoku(buffer []byte, n int, c mocknet.UDPConn, mnet *mocknet.MockNet, addr *net.UDPAddr, addrKey addrKey, localMappings map[int]addrKey) bool {
	if !soku {
		return false
	}
	if soku {
		if n >= 14 && buffer[1] == 0x08 && buffer[6] == 0x02 && buffer[7] == 0x00 && net.IP(buffer[10:14]).Equal(net.IPv4(127, 0, 0, 1)) {
			localPort := mnet.ActualPort(int(binary.BigEndian.Uint16(buffer[8:10])))
			if peerAddr, ok := localMappings[localPort]; ok {
				// send proxypunch redirect with soku redirect body
				copy(buffer[7:], buffer[1:n+1])
				buffer[0] = 0xCE
				buffer[1] = byte(peerAddr.port >> 8)
				buffer[2] = byte(peerAddr.port)
				copy(buffer[3:7], peerAddr.ip[:])
				c.WriteToUDP(buffer[:7+n], addr)
				// also send proxypunch create mapping to force punch on client
				buffer[0] = 0xCF
				buffer[1] = byte(addrKey.port >> 8)
				buffer[2] = byte(addrKey.port)
				copy(buffer[3:7], addrKey.ip[:])
				c.WriteToUDP(buffer[:7], peerAddr.toUDP())
				return true
			}
		}
		if n >= 9 && buffer[1] == 0x02 && buffer[2] == 0x02 && buffer[3] == 0x00 && net.IP(buffer[6:10]).Equal(net.IPv4(127, 0, 0, 1)) {
			localPort := mnet.ActualPort(int(binary.BigEndian.Uint16(buffer[4:6])))
			if peerAddr, ok := localMappings[localPort]; ok {
				// drop soku redirect packet, send proxypunch create mapping instead
				buffer[0] = 0xCF
				buffer[1] = byte(peerAddr.port >> 8)
				buffer[2] = byte(peerAddr.port)
				copy(buffer[3:7], peerAddr.ip[:])
				c.WriteToUDP(buffer[:7], addr)
				return true
			}
		}
	}
	return false
}

/*

if soku {
						if n >= 13 && buffer[1] == 0x08 && buffer[6] == 0x02 && buffer[7] == 0x00 && net.IP(buffer[10:14]).Equal(net.IPv4(127, 0, 0, 1)) {
							localPort := mnet.ActualPort(int(binary.BigEndian.Uint16(buffer[8:10])))
							if peerAddr, ok := localMappings[localPort]; ok {
								// send proxypunch redirect with soku redirect body
								copy(buffer[7:], buffer[1:n+1])
								buffer[0] = 0xCE
								buffer[1] = byte(peerAddr.port >> 8)
								buffer[2] = byte(peerAddr.port)
								copy(buffer[3:7], peerAddr.ip[:])
								c.WriteToUDP(buffer[:7+n], addr)
								// also send proxypunch create mapping to force punch on client
								buffer[0] = 0xCF
								buffer[1] = byte(addrKey.port >> 8)
								buffer[2] = byte(addrKey.port)
								copy(buffer[3:7], addrKey.ip[:])
								c.WriteToUDP(buffer[:7], peerAddr.toUDP())
								continue
							}
						}
						if buffer[1] == 0x02 {
							fmt.Println("lol") // FIXME
						}
						if n >= 9 && buffer[1] == 0x02 && buffer[2] == 0x02 && buffer[3] == 0x00 && net.IP(buffer[6:10]).Equal(net.IPv4(127, 0, 0, 1)) {
							localPort := mnet.ActualPort(int(binary.BigEndian.Uint16(buffer[4:6])))
							if peerAddr, ok := localMappings[localPort]; ok {
								// drop soku redirect packet, send proxypunch create mapping instead
								buffer[0] = 0xCF
								buffer[1] = byte(peerAddr.port >> 8)
								buffer[2] = byte(peerAddr.port)
								copy(buffer[3:7], peerAddr.ip[:])
								c.WriteToUDP(buffer[:7], addr)
								continue
							}
						}
					}

*/
