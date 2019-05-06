package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

const version = "0.1.1"

const defaultPort = 14762

const flushInterval = 15 * time.Second

type key struct {
	ip   [4]byte
	port int
}

type serverValue struct {
	natPort int
	time    time.Time
}

type clientValue struct {
	localIp [4]byte
	natPort int
	time    time.Time
}

func main() {
	fmt.Println("proxypunch relay v" + version)
	fmt.Println()

	var port int
	flag.IntVar(&port, "port", defaultPort, "relay listen port")
	flag.Parse()

	c, err := net.ListenUDP("udp4", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	servers := make(map[key]serverValue)
	clients := make(map[key]map[clientValue]struct{})

	flushTime := time.Now()

	buffer := make([]byte, 4096)
	for {
		now := time.Now()
		if now.Sub(flushTime) > flushInterval {
			flushTime = now
			for k, v := range servers {
				if now.Sub(v.time) > flushInterval {
					delete(servers, k)
				}
			}
			for k, v := range clients {
				for k_ := range v {
					if now.Sub(k_.time) > flushInterval {
						delete(v, k_)
					}
				}
				if len(v) == 0 {
					delete(clients, k)
				}
			}
		}
		n, addr, err := c.ReadFromUDP(buffer)
		if err != nil {
			// err is thrown if the buffer is too small
			continue
		}
		if n != 2 && n != 6 {
			continue
		}
		var senderIp [4]byte
		if senderIpSlice := addr.IP.To4(); senderIpSlice == nil {
			continue
		} else {
			copy(senderIp[:], senderIpSlice)
		}
		if n == 2 {
			key := key{
				ip:   senderIp,
				port: int(binary.BigEndian.Uint16(buffer[:2])),
			}
			servers[key] = serverValue{
				natPort: addr.Port,
				time:    time.Now(),
			}
			for i, v := range senderIp {
				buffer[i] = v ^ 0xCC // xor ip to avoid nat rewriting it
			}
			n := 4
			if v, ok := clients[key]; ok {
				for k := range v {
					if n+6 > 4096 {
						break
					}
					buffer[n] = byte(k.natPort >> 8)
					buffer[n+1] = byte(k.natPort)
					copy(buffer[n+2:], k.localIp[:])
					n += 6
				}
			}
			c.WriteToUDP(buffer[:n], addr)
		} else if n == 6 {
			var ip [4]byte
			copy(ip[:], buffer[2:])
			key := key{
				ip:   ip,
				port: int(binary.BigEndian.Uint16(buffer[:2])),
			}
			v := clientValue{
				localIp: senderIp,
				natPort: addr.Port,
				time:    time.Now(),
			}
			if c, ok := clients[key]; !ok {
				clients[key] = make(map[clientValue]struct{}, 4)
				clients[key][v] = struct{}{}
			} else {
				c[v] = struct{}{}
			}
			if val, ok := servers[key]; ok {
				serverPayload := []byte{byte(val.natPort >> 8), byte(val.natPort)}
				c.WriteToUDP(serverPayload, addr)
			}
		}
	}
}
