package domain_test

import (
	"regexp"
	"testing"

	"inmo.platform/contexts/contracts/internal/domain"
)

var uuidLikePattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestNewContractActivated_MapeaCamposCorrectamente(t *testing.T) {
	evt := domain.NewContractActivated("contract-1", "prop-1")

	if evt.AggregateID() != "contract-1" {
		t.Fatalf("aggregate id: got %s, want %s", evt.AggregateID(), "contract-1")
	}
	if evt.PropertyID != "prop-1" {
		t.Fatalf("property id: got %s, want %s", evt.PropertyID, "prop-1")
	}
	if evt.EventName() != "contracts.contract.activated" {
		t.Fatalf("event name: got %s, want %s", evt.EventName(), "contracts.contract.activated")
	}
	if evt.OccurredAt().IsZero() {
		t.Fatal("OccurredAt no debería estar vacío")
	}
	if !uuidLikePattern.MatchString(evt.EventID()) {
		t.Fatalf("event id no tiene forma de UUID: %q", evt.EventID())
	}
}

func TestNewContractActivated_CadaLlamadaGeneraUnEventIDDistinto(t *testing.T) {
	evt1 := domain.NewContractActivated("contract-1", "prop-1")
	evt2 := domain.NewContractActivated("contract-1", "prop-1")

	if evt1.EventID() == evt2.EventID() {
		t.Fatal("dos eventos distintos no deberían compartir EventID")
	}
}
