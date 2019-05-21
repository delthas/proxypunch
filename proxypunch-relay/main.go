package main

import (
	"flag"
	"fmt"

	"github.com/delthas/proxypunch/mocknet"
	"github.com/delthas/proxypunch/proxypunch-relay/relay"
)

const version = "0.1.1"

const defaultPort = 14762

func main() {
	fmt.Println("proxypunch relay v" + version)
	fmt.Println()

	var port int
	flag.IntVar(&port, "port", defaultPort, "relay listen port")
	flag.Parse()

	relay.Relay(&mocknet.MockNet{}, port)
}
