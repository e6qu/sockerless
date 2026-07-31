package aws_sdk_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFreeTCPUDPPortSupportsRoute53DualProtocolBind(t *testing.T) {
	port, err := freeTCPUDPPort()
	require.NoError(t, err)

	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tcpListener.Close()) })

	udpListener, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, udpListener.Close()) })
}
