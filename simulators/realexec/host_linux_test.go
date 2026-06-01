//go:build realexec_host && linux

package realexec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHostCapabilitiesForRealExecution(t *testing.T) {
	report := DetectCapabilities("firecracker", "jailer", "ip", "nft")
	if err := report.Require(); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkNamespaceNICRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	prefix := shortPrefix()
	host := NewHost()
	network, err := host.CreateNetwork(ctx, NetworkSpec{
		NamespaceName: prefix + "nw",
		BridgeName:    prefix + "br",
		CIDR:          "10.203.0.0/29",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := network.Close(context.Background()); err != nil {
			t.Fatalf("network cleanup: %v", err)
		}
	}()

	first, err := network.AttachNamespaceNIC(ctx, NamespaceNICSpec{
		NamespaceName: prefix + "n1",
		HostVethName:  prefix + "h1",
		GuestVethName: prefix + "g1",
		MAC:           "02:00:5e:10:00:01",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := first.Close(context.Background()); err != nil {
			t.Fatalf("first NIC cleanup: %v", err)
		}
	}()

	second, err := network.AttachNamespaceNIC(ctx, NamespaceNICSpec{
		NamespaceName: prefix + "n2",
		HostVethName:  prefix + "h2",
		GuestVethName: prefix + "g2",
		MAC:           "02:00:5e:10:00:02",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := second.Close(context.Background()); err != nil {
			t.Fatalf("second NIC cleanup: %v", err)
		}
	}()

	if first.PrivateIP.Equal(second.PrivateIP) {
		t.Fatalf("duplicate private IP lease: %s", first.PrivateIP)
	}
	if first.PrivateIP.String() != "10.203.0.2" || second.PrivateIP.String() != "10.203.0.3" {
		t.Fatalf("leases = %s, %s; want 10.203.0.2, 10.203.0.3", first.PrivateIP, second.PrivateIP)
	}

	runner := Runner{}
	if _, err := runner.Output(ctx, "ip", "netns", "exec", network.NamespaceName, "ip", "link", "show", network.BridgeName); err != nil {
		t.Fatalf("network bridge %s is not inside namespace %s: %v", network.BridgeName, network.NamespaceName, err)
	}
	if err := runner.Run(ctx, "ip", "netns", "exec", first.NamespaceName, "ping", "-c", "1", "-W", "1", network.Gateway.String()); err != nil {
		t.Fatalf("first namespace cannot reach bridge gateway %s: %v", network.Gateway, err)
	}
	if err := runner.Run(ctx, "ip", "netns", "exec", second.NamespaceName, "ping", "-c", "1", "-W", "1", first.PrivateIP.String()); err != nil {
		t.Fatalf("second namespace cannot reach first NIC %s over bridge: %v", first.PrivateIP, err)
	}

	table := prefix + "tbl"
	if err := runner.Run(ctx, "nft", "add", "table", "inet", table); err != nil {
		t.Fatalf("create nft cleanup target: %v", err)
	}
	var cleanup CleanupStack
	cleanup.Add(func(cleanupCtx context.Context) error {
		return runner.Run(cleanupCtx, "nft", "delete", "table", "inet", table)
	})
	if err := cleanup.Close(ctx); err != nil {
		t.Fatalf("cleanup nft table: %v", err)
	}
	if _, err := runner.Output(ctx, "nft", "list", "table", "inet", table); err == nil {
		t.Fatalf("nft table %s still exists after cleanup", table)
	}
}

func shortPrefix() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sx%06x", os.Getpid())[:8]
	}
	return "sx" + hex.EncodeToString(b[:])
}

func TestCloseRemovesHostArtifacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prefix := shortPrefix()
	host := NewHost()
	network, err := host.CreateNetwork(ctx, NetworkSpec{
		NamespaceName: prefix + "nw",
		BridgeName:    prefix + "br",
		CIDR:          "10.204.0.0/29",
	})
	if err != nil {
		t.Fatal(err)
	}
	nic, err := network.AttachNamespaceNIC(ctx, NamespaceNICSpec{
		NamespaceName: prefix + "ns",
		HostVethName:  prefix + "hv",
		GuestVethName: prefix + "gv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := nic.Close(ctx); err != nil {
		t.Fatalf("close nic: %v", err)
	}
	if err := network.Close(ctx); err != nil {
		t.Fatalf("close network: %v", err)
	}

	runner := Runner{}
	out, err := runner.Output(ctx, "ip", "netns", "list")
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	if strings.Contains(out, prefix+"ns") {
		t.Fatalf("namespace %sns still exists after cleanup: %s", prefix, out)
	}
	if strings.Contains(out, prefix+"nw") {
		t.Fatalf("network namespace %snw still exists after cleanup: %s", prefix, out)
	}
}
