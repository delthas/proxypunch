package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

const version = "0.0.1"

const defaultPort = 14761

const flushInterval = 15 * time.Second

type key struct {
	ip   [4]byte
	port int
}

type clientValue struct {
	localIp [4]byte
	natPort int
	time    time.Time
}

type serverValue struct {
	natPort int
	time    time.Time
}

func main() {
	fmt.Println("ProxyPunch Server v" + version)
	fmt.Println()

	var port int
	flag.IntVar(&port, "port", defaultPort, "server listen port")
	flag.Parse()

	c, err := net.ListenUDP("udp4", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	clients := make(map[key]clientValue)
	servers := make(map[key]serverValue)

	flushTime := time.Now()

	buffer := make([]byte, 6)
	for {
		now := time.Now()
		if now.Sub(flushTime) > flushInterval {
			flushTime = now
			for k, v := range clients {
				if now.Sub(v.time) > flushInterval {
					delete(clients, k)
				}
			}
			for k, v := range servers {
				if now.Sub(v.time) > flushInterval {
					delete(servers, k)
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
			if val, ok := clients[key]; ok {
				serverPayload := append([]byte{byte(val.natPort >> 8), byte(val.natPort)}, val.localIp[:]...)
				c.WriteToUDP(serverPayload, addr)
			}
		} else if n == 6 {
			var ip [4]byte
			copy(ip[:], buffer[2:])
			key := key{
				ip:   ip,
				port: int(binary.BigEndian.Uint16(buffer[:2])),
			}
			clients[key] = clientValue{
				localIp: senderIp,
				natPort: addr.Port,
				time:    time.Now(),
			}
			if val, ok := servers[key]; ok {
				serverPayload := append([]byte{byte(val.natPort >> 8), byte(val.natPort)})
				c.WriteToUDP(serverPayload, addr)
			}
		}
	}
}
