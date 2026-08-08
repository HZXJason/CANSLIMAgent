package handlers

import (
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestScreenDefinitionRequiresVisibleRulesAndUnknownSemantics(t *testing.T) {
	definition := models.DefaultCANSLIMScreenDefinition()
	definition.Name = "我的盘前模板"
	definition.IsSystem = false
	if got := validateScreenDefinition(definition); got != "" {
		t.Fatalf("valid template rejected: %s", got)
	}
	definition.Conditions[0].UnknownPolicy = "pass"
	if got := validateScreenDefinition(definition); got != "unknown data must remain unknown" {
		t.Fatalf("unexpected validation: %s", got)
	}
}
