package mocknet

import (
	"testing"

	"github.com/delthas/proxypunch/mocknet"

	"github.com/delthas/proxypunch/punch"

	"github.com/delthas/proxypunch/proxypunch-relay/relay"
)

func TestPunch(t *testing.T) {
	test := mocknet.NewTest()
	r := test.NewNet(1, false)
	server := test.NewNet(2, true)
	client1 := test.NewNet(3, true)
	client2 := test.NewNet(4, true)
	client3 := test.NewNet(5, true)
	go func() {
		relay.Relay(r, 25678)
	}()
	go func() {
		punch.Server(server, "10.0.0.1:25678", 32145)
	}()
	go func() {
		punch.Client(client1, "10.0.0.1:25678", "10.0.0.2", 32145)
	}()
	go func() {
		punch.Client(client2, "10.0.0.1:25678", "10.0.0.2", 32145)
	}()
	go func() {
		punch.Client(client3, "10.0.0.1:25678", "10.0.0.2", 32145)
	}()
	select {}
}
