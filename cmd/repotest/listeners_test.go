package main

import (
	"net"
	"testing"
)

func TestBindListenersClosesLocalWhenLANBindFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	localProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localAddr := localProbe.Addr().String()
	if err = localProbe.Close(); err != nil {
		t.Fatal(err)
	}
	local, lan, err := bindListeners(localAddr, occupied.Addr().String())
	if err == nil {
		if local != nil {
			local.Close()
		}
		if lan != nil {
			lan.Close()
		}
		t.Fatal("expected LAN bind failure")
	}
	if local != nil || lan != nil {
		t.Fatal("failed startup returned an open listener")
	}
	retry, err := net.Listen("tcp", localAddr)
	if err != nil {
		t.Fatalf("loopback listener leaked after LAN bind failure: %v", err)
	}
	retry.Close()
}

func TestBindListenersReturnsBothUnservedListeners(t *testing.T) {
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localAddr := local.Addr().String()
	local.Close()
	lan, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lanAddr := lan.Addr().String()
	lan.Close()
	l, ln, err := bindListeners(localAddr, lanAddr)
	if err != nil {
		t.Fatal(err)
	}
	if l == nil || ln == nil {
		t.Fatal("both listeners must be bound before serving")
	}
	defer l.Close()
	defer ln.Close()
}

func TestBindListenersRejectsHostnameForLocalAddress(t *testing.T) {
	local, lan, err := bindListeners("localhost:0", "")
	if local != nil {
		local.Close()
	}
	if lan != nil {
		lan.Close()
	}
	if err == nil {
		t.Fatal("hostname bind could resolve outside loopback and must be rejected")
	}
}

func TestBindListenersRejectsExternalLocalAddress(t *testing.T) {
	local, lan, err := bindListeners("0.0.0.0:0", "")
	if local != nil {
		local.Close()
	}
	if lan != nil {
		lan.Close()
	}
	if err == nil {
		t.Fatal("non-loopback address accepted for local management")
	}
}
