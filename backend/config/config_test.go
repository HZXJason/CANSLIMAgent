package config

import "testing"

func TestLoadBrokerGatewaysAcceptsLoopbackHosts(t *testing.T) {
	t.Setenv("BROKER_FUTU_HOST", "localhost")
	t.Setenv("BROKER_FUTU_PORT", "11112")
	t.Setenv("BROKER_IBKR_HOST", "::1")
	t.Setenv("BROKER_IBKR_PORT", "7498")

	cfg := Load()

	if got, want := cfg.BrokerFutuGateway.Host, "localhost"; got != want {
		t.Fatalf("Futu host = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerFutuGateway.Port, "11112"; got != want {
		t.Fatalf("Futu port = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Host, "::1"; got != want {
		t.Fatalf("IBKR host = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Port, "7498"; got != want {
		t.Fatalf("IBKR port = %q, want %q", got, want)
	}
}

func TestLoadBrokerGatewaysRejectsRemoteHostsAndInvalidPorts(t *testing.T) {
	t.Setenv("BROKER_FUTU_HOST", "gateway.example.com")
	t.Setenv("BROKER_FUTU_PORT", "not-a-port")
	t.Setenv("BROKER_IBKR_HOST", "192.168.1.50")
	t.Setenv("BROKER_IBKR_PORT", "70000")

	cfg := Load()

	if got, want := cfg.BrokerFutuGateway.Host, "127.0.0.1"; got != want {
		t.Fatalf("Futu host = %q, want safe default %q", got, want)
	}
	if got, want := cfg.BrokerFutuGateway.Port, "11111"; got != want {
		t.Fatalf("Futu port = %q, want safe default %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Host, "127.0.0.1"; got != want {
		t.Fatalf("IBKR host = %q, want safe default %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Port, "7497"; got != want {
		t.Fatalf("IBKR port = %q, want safe default %q", got, want)
	}
}
