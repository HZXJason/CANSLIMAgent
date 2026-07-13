package config

import "testing"

func TestLoadUsesLoopbackAndNoBrowserTrustByDefault(t *testing.T) {
	t.Setenv("HOST", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("API_AUTH_TOKEN", "")
	t.Setenv("SIDECAR_SHARED_SECRET", "")

	cfg := Load()
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want loopback", cfg.Host)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
}

func TestProductionDisablesInProcessMigrationsByDefault(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("RUN_DATABASE_MIGRATIONS", "")
	t.Setenv("MIGRATE_ONLY", "")

	cfg := Load()
	if cfg.RunDatabaseMigrations {
		t.Fatal("production API process enabled schema migrations by default")
	}
	if cfg.MigrateOnly {
		t.Fatal("production API process unexpectedly entered migration-only mode")
	}
}

func TestLoadLongPortOAuthUsesPublicPKCEDefaults(t *testing.T) {
	t.Setenv("BROKER_LONGPORT_CLIENT_ID", "public-client-id")
	t.Setenv("BROKER_LONGPORT_CLIENT_SECRET", "")
	t.Setenv("BROKER_LONGPORT_AUTHORIZE_URL", "")
	t.Setenv("BROKER_LONGPORT_TOKEN_URL", "")
	t.Setenv("BROKER_LONGPORT_REDIRECT_URL", "")
	t.Setenv("BROKER_LONGPORT_SCOPES", "")

	cfg := Load()
	if !cfg.BrokerLongPortOAuth.Enabled() {
		t.Fatal("public PKCE client must be enabled without a client secret")
	}
	if got, want := cfg.BrokerLongPortOAuth.AuthorizeURL, "https://openapi.longbridge.com/oauth2/authorize"; got != want {
		t.Fatalf("authorize URL = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerLongPortOAuth.TokenURL, "https://openapi.longbridge.com/oauth2/token"; got != want {
		t.Fatalf("token URL = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerLongPortOAuth.RedirectURL, "http://127.0.0.1:8080/api/v1/brokers/oauth/longport/callback"; got != want {
		t.Fatalf("redirect URL = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerLongPortOAuth.Scopes, []string{"3"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("scopes = %#v, want %#v", got, want)
	}
}

func TestLoadBrokerGatewaysAcceptsLoopbackHosts(t *testing.T) {
	t.Setenv("BROKER_FUTU_HOST", "localhost")
	t.Setenv("BROKER_FUTU_PORT", "11112")
	t.Setenv("BROKER_FUTU_ACCOUNT", "FUTU-001")
	t.Setenv("BROKER_IBKR_HOST", "::1")
	t.Setenv("BROKER_IBKR_PORT", "7498")
	t.Setenv("BROKER_IBKR_ACCOUNT", "DU123456")

	cfg := Load()

	if got, want := cfg.BrokerFutuGateway.Host, "localhost"; got != want {
		t.Fatalf("Futu host = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerFutuGateway.Port, "11112"; got != want {
		t.Fatalf("Futu port = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerFutuGateway.Account, "FUTU-001"; got != want {
		t.Fatalf("Futu account = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Host, "::1"; got != want {
		t.Fatalf("IBKR host = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Port, "7498"; got != want {
		t.Fatalf("IBKR port = %q, want %q", got, want)
	}
	if got, want := cfg.BrokerIBKRGateway.Account, "DU123456"; got != want {
		t.Fatalf("IBKR account = %q, want %q", got, want)
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
