package main

import "testing"

func TestDemoServerRequiresLoopbackUnlessExternalIsAllowed(t *testing.T) {
	if _, err := demoServer(options{address: "127.0.0.1:8090"}); err != nil {
		t.Fatalf("loopback demo address was rejected: %v", err)
	}
	if _, err := demoServer(options{address: "0.0.0.0:8090"}); err == nil {
		t.Fatal("non-loopback demo address was accepted without the opt-in")
	}
	if _, err := demoServer(options{address: "0.0.0.0:8090", demoAllowExternal: true}); err != nil {
		t.Fatalf("explicitly allowed demo address was rejected: %v", err)
	}
	if _, err := demoServer(options{address: "8090", demoAllowExternal: true}); err == nil {
		t.Fatal("address without a port was accepted")
	}
}
