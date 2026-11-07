//go:build !windows

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"testing"
)

func TestSocketOwner(t *testing.T) {
	// the way /proc/net/tcp writes addresses: groups of 4 bytes as numbers in this computer's order
	group := func(b ...byte) uint32 { return binary.NativeEndian.Uint32(b) }
	v4 := fmt.Sprintf("%08X:%04X", group(127, 0, 0, 1), 8080)
	v6 := fmt.Sprintf("%08X%08X%08X%08X:%04X", group(0, 0, 0, 0), group(0, 0, 0, 0), group(0, 0, 0xff, 0xff), group(127, 0, 0, 1), 8080)
	lo := net.ParseIP("127.0.0.1")
	if !procAddrIs(v4, lo, 8080) || !procAddrIs(v6, lo, 8080) || procAddrIs(v4, lo, 8081) || procAddrIs(v4, net.ParseIP("127.0.0.2"), 8080) || procAddrIs("zz:1F90", lo, 8080) {
		t.Fatal("addresses read wrongly")
	}
	if _, err := os.Stat("/proc/net/tcp"); err != nil {
		t.Skip("no /proc/net/tcp here")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// seen from the server: the far end is this test's own connection
	if uid, ok := socketOwner(s.RemoteAddr().String(), s.LocalAddr().String()); !ok || uid != os.Getuid() {
		t.Fatalf("owner %d %v, want %d", uid, ok, os.Getuid())
	}
	if _, ok := socketOwner("127.0.0.1:1", ln.Addr().String()); ok {
		t.Fatal("found a connection that is not there")
	}
}
