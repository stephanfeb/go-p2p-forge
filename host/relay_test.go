package host_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"

	forgehost "github.com/stephanfeb/go-p2p-forge/host"
)

const relayTestProto = "/forge-test/relay/1.0.0"

// relayedTransfer has a peer reserve a slot on a relay built by Create,
// has a second peer send payload to it through the relay, and returns what
// arrived and whether the relayed connection was limited.
func relayedTransfer(t *testing.T, limits forgehost.RelayLimits, payload []byte) (received []byte, limited bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg := forgehost.DefaultConfig()
	cfg.ListenAddresses = []string{"/ip4/127.0.0.1/tcp/0"}
	cfg.EnableAutoNAT = false
	cfg.EnableRelayService = true
	cfg.RelayLimits = limits
	relay, err := forgehost.Create(cfg, testKey(t), silentLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	relayInfo := peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}

	newPeer := func() host.Host {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.EnableRelay())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { h.Close() })
		if err := h.Connect(ctx, relayInfo); err != nil {
			t.Fatal(err)
		}
		return h
	}
	listener, dialer := newPeer(), newPeer()

	got := make(chan []byte, 1)
	listener.SetStreamHandler(relayTestProto, func(s network.Stream) {
		data, _ := io.ReadAll(s)
		got <- data
		s.Close()
	})
	if _, err := client.Reserve(ctx, listener, relayInfo); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	circuit, err := ma.NewMultiaddr(relay.Addrs()[0].String() + "/p2p/" + relay.ID().String() + "/p2p-circuit")
	if err != nil {
		t.Fatal(err)
	}
	if err := dialer.Connect(ctx, peer.AddrInfo{ID: listener.ID(), Addrs: []ma.Multiaddr{circuit}}); err != nil {
		t.Fatalf("connect through the relay: %v", err)
	}
	s, err := dialer.NewStream(network.WithAllowLimitedConn(ctx, "test"), listener.ID(), relayTestProto)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	limited = s.Conn().Stat().Limited
	_, _ = s.Write(payload)
	_ = s.CloseWrite()

	select {
	case received = <-got:
	case <-ctx.Done():
		t.Fatal("listener never finished reading")
	}
	return received, limited
}

// A relayed connection is cut at go-libp2p's 128 KiB unless the relay is
// told otherwise; UnlimitedConnections lifts the cut and the limited mark.
func TestRelayConnectionLimits(t *testing.T) {
	payload := bytes.Repeat([]byte("r"), 256<<10)

	t.Run("default cuts at 128 KiB", func(t *testing.T) {
		got, limited := relayedTransfer(t, forgehost.RelayLimits{}, payload)
		if !limited {
			t.Error("relayed connection not marked limited under the default limits")
		}
		if len(got) > 128<<10 {
			t.Errorf("received %d bytes, want at most the 128 KiB default", len(got))
		}
	})

	t.Run("configured data limit applies", func(t *testing.T) {
		got, _ := relayedTransfer(t, forgehost.RelayLimits{ConnectionData: 32 << 10}, payload)
		if len(got) > 32<<10 {
			t.Errorf("received %d bytes, want at most the configured 32 KiB", len(got))
		}
	})

	t.Run("unlimited carries everything", func(t *testing.T) {
		got, limited := relayedTransfer(t, forgehost.RelayLimits{
			UnlimitedConnections: true,
			ConnectionData:       32 << 10, // ignored
		}, payload)
		if limited {
			t.Error("unlimited relayed connection marked limited")
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("received %d of %d bytes", len(got), len(payload))
		}
	})
}
