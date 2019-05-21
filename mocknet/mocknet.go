package mocknet

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

// external ip is 10.0.0.<network>:<optionally nated port>

func NewTest() MockTest {
	return MockTest{
		networks: make(map[int]*MockNet),
	}
}

type mockData struct {
	data   []byte
	sender *net.UDPAddr
}

type MockTest struct {
	networks map[int]*MockNet
}

func (t *MockTest) NewNet(id int, nat bool) *MockNet {
	var mappings map[int]*natMapping
	if nat {
		mappings = make(map[int]*natMapping)
	}
	if _, ok := t.networks[id]; ok {
		panic("network already created")
	}
	n := &MockNet{
		test:        t,
		mock:        true,
		id:          id,
		nat:         mappings,
		connections: make(map[int]*mockConn),
	}
	t.networks[id] = n
	return n
}

type natRemoteAddr struct {
	id   int
	port int
}

type natMapping struct {
	mutex     sync.Mutex
	addresses map[natRemoteAddr]time.Time
	natPort   int
}

type MockNet struct {
	test            *MockTest
	mock            bool
	id              int
	nat             map[int]*natMapping
	connections     map[int]*mockConn
	connectionMutex sync.Mutex
}

type UDPConn interface {
	LocalAddr() net.Addr
	ReadFromUDP(b []byte) (int, *net.UDPAddr, error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (int, error)
	Close() error
}

type mockConn struct {
	network   *MockNet
	port      int
	closed    bool
	closeChan chan struct{}
	readChan  chan mockData
	proxy     *net.UDPConn
}

func (c *mockConn) LocalAddr() net.Addr {
	return &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: c.port,
	}
}

func (c *mockConn) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	if c.closed {
		return 0, nil, errors.New("connection is already closed")
	}
	select {
	case <-c.closeChan:
		return 0, nil, errors.New("connection is closed")
	case data := <-c.readChan:
		if len(b) < len(data.data) {
			return 0, nil, errors.New("buffer is not large enough")
		}
		copy(b, data.data)
		return len(data.data), data.sender, nil
	}
}

func (c *mockConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {
	if addr.IP.To4() == nil {
		panic("ip is ipv6: " + addr.IP.String())
	}
	ip := net.IP(make([]byte, 4))
	copy(ip, addr.IP.To4())
	if c.closed {
		return 0, errors.New("connection is already closed")
	}
	if ip[0] == 127 {
		if ip[1] == 1 {
			ip = []byte{192, 168, ip[2], ip[3]}
		} else if ip[1] == 2 && ip[2] == 0 && ip[3] == 1 {
			ip = []byte{127, 0, 0, 1}
		} else if ip[1] == 0 && ip[2] == 0 && ip[3] == 1 {
			ip = []byte{127, 0, 0, 1}
		} else {
			panic("unknown ip: " + ip.String())
		}
		// fmt.Fprintf(os.Stderr, ">>>sending from %d to proxy (%s) %s:%d\n", c.network.id, c.proxy.LocalAddr().String(), ip.String(), addr.Port)
		_, err := c.proxy.WriteToUDP(b, &net.UDPAddr{
			IP:   ip,
			Port: addr.Port,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error while sending packet to proxy: %s\n", err.Error())
		}
	} else {
		if ip[0] != 10 || ip[1] != 0 || ip[2] != 0 {
			panic("trying to send to unknown ip prefix")
		}
		id := int(ip[3])
		if id == c.network.id {
			panic("trying to send by hairpinning connection")
		}
		network, ok := c.network.test.networks[id]
		if !ok {
			panic("trying to unexistant network")
		}
		// fmt.Fprintf(os.Stderr, ">>>sending from %d to %d\n", c.network.id, network.id)
		var externalPort int
		if c.network.nat == nil {
			externalPort = c.port
		} else {
			var mapping *natMapping
			if m, ok := c.network.nat[c.port]; ok {
				mapping = m
				externalPort = mapping.natPort
			} else {
			portGen:
				for {
					externalPort = 40000 + rand.Intn(10000)
					for i, mapping := range c.network.nat {
						if i == externalPort || mapping.natPort == externalPort {
							continue portGen
						}
					}
					break
				}
				mapping = &natMapping{
					addresses: make(map[natRemoteAddr]time.Time),
					natPort:   externalPort,
				}
				c.network.nat[c.port] = mapping
			}
			mapping.mutex.Lock()
			mapping.addresses[natRemoteAddr{id, addr.Port}] = time.Now()
			mapping.mutex.Unlock()
		}
		internalPort := -1
		if network.nat == nil {
			internalPort = addr.Port
		} else {
		outer:
			for i, mapping := range network.nat {
				if mapping.natPort != addr.Port {
					continue
				}
				mapping.mutex.Lock()
				for addr, last := range mapping.addresses {
					if addr.id != c.network.id || addr.port != externalPort {
						continue
					}
					if time.Now().Sub(last) > 30*time.Second {
						delete(mapping.addresses, addr)
						if len(mapping.addresses) == 0 {
							delete(network.nat, i)
						}
						fmt.Fprintf(os.Stderr, "dropped packet sent to expired natted connection from %d to %d\n", c.network.id, network.id)
						mapping.mutex.Unlock()
						return len(b), nil
					}
					internalPort = i
					mapping.mutex.Unlock()
					break outer
				}
				fmt.Fprintf(os.Stderr, "dropped packet sent to natted ip from new address from %d to %d\n", c.network.id, network.id)
				mapping.mutex.Unlock()
				return len(b), nil
			}
		}
		network.connectionMutex.Lock()
		conn, ok := network.connections[internalPort]
		network.connectionMutex.Unlock()
		if ok {
			if conn.closed {
				fmt.Fprintf(os.Stderr, "dropped packet sent to remote closed conn from %d to %d\n", c.network.id, network.id)
			} else {
				data := make([]byte, len(b))
				copy(data, b)
				conn.readChan <- mockData{
					data: data,
					sender: &net.UDPAddr{
						IP:   net.IPv4(10, 0, 0, byte(c.network.id)).To4(),
						Port: externalPort,
					},
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "dropped packet sent to remote unknown conn from %d to %d\n", c.network.id, network.id)
		}
	}
	return len(b), nil
}

func (c *mockConn) Close() error {
	if c.closed {
		fmt.Fprintf(os.Stderr, "connection already closed\n")
	}
	c.closed = true
	c.closeChan <- struct{}{}
	c.proxy.Close()
	c.network.connectionMutex.Lock()
	delete(c.network.connections, c.port)
	c.network.connectionMutex.Unlock()
	return nil
}

func (m *MockNet) ListenUDP(network string, laddr *net.UDPAddr) (UDPConn, error) {
	if !m.mock {
		return net.ListenUDP(network, laddr)
	}
	if network != "udp4" {
		panic("unknown network")
	}
	var port int
	if laddr == nil || laddr.Port == 0 {
		for {
			port = 50000 + rand.Intn(10000)
			m.connectionMutex.Lock()
			_, ok := m.connections[port]
			m.connectionMutex.Unlock()
			if !ok {
				break
			}
		}
	} else {
		port = laddr.Port
		m.connectionMutex.Lock()
		_, ok := m.connections[port]
		m.connectionMutex.Unlock()
		if ok {
			return nil, errors.New("connection already exists")
		}
	}
	cProxy, err := net.ListenUDP("udp4", nil)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "proxy of %d %d is 127.0.0.1:%d\n", m.id, port, cProxy.LocalAddr().(*net.UDPAddr).Port)
	c := &mockConn{
		network:   m,
		port:      port,
		closed:    false,
		closeChan: make(chan struct{}, 100),
		readChan:  make(chan mockData, 100),
		proxy:     cProxy,
	}
	go func() {
		buf := make([]byte, 4096)
		for !c.closed {
			n, addr, err := cProxy.ReadFromUDP(buf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error while reading from proxy connection of %d %d: %s\n", m.id, port, err.Error())
				break
			}
			// fmt.Fprintf(os.Stderr, "<<<reading from proxy( %s) %s to %d\n", cProxy.LocalAddr().String(), addr.String(), m.id)
			ip := net.IP(make([]byte, 4))
			copy(ip, addr.IP.To4())
			if ip[0] == 192 && ip[1] == 168 {
				ip = []byte{127, 1, ip[2], ip[3]}
			} else if ip[0] == 127 && ip[1] == 0 && ip[2] == 0 && ip[3] == 1 {
				ip[1] = 2
			} else {
				panic("unknown ip: " + ip.String())
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			c.readChan <- mockData{
				data:   data,
				sender: &net.UDPAddr{IP: ip, Port: addr.Port},
			}
		}
	}()
	m.connectionMutex.Lock()
	m.connections[port] = c
	m.connectionMutex.Unlock()
	return c, nil
}

func (m *MockNet) ActualPort(proxyPort int) int {
	if !m.mock {
		return proxyPort
	}
	m.connectionMutex.Lock()
	defer m.connectionMutex.Unlock()
	for port, c := range m.connections {
		if c.proxy.LocalAddr().(*net.UDPAddr).Port == proxyPort {
			return port
		}
	}
	return -1
}
