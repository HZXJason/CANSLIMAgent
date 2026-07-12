package models_test

import (
	"testing"

	"github.com/alphalens/canslimagent/backend/config"
	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestBrokerConnectionMetadataUsesOnlyKnownStates(t *testing.T) {
	if !models.ValidBrokerConnectionStatus(models.BrokerConnectionStatusConnected) {
		t.Fatal("connected must be valid")
	}
	if models.ValidBrokerConnectionStatus("browser_supplied_token") {
		t.Fatal("unknown state accepted")
	}
}

func TestBrokerOAuthConfigIsDisabledWithoutClientID(t *testing.T) {
	if (config.BrokerOAuthConfig{}).Enabled() {
		t.Fatal("empty config must be disabled")
	}
}
