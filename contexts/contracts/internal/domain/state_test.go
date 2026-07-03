package domain_test

import (
	"testing"

	"inmo.platform/contexts/contracts/internal/domain"
)

func TestContractState_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from domain.ContractState
		to   domain.ContractState
		want bool
	}{
		// Desde DRAFT solo se puede pasar a ACTIVE (firma del contrato).
		{domain.StateDraft, domain.StateActive, true},
		{domain.StateDraft, domain.StateDraft, false},
		{domain.StateDraft, domain.StateRenewed, false},
		{domain.StateDraft, domain.StateTerminated, false},

		// Desde ACTIVE se puede renovar o rescindir, pero no volver a DRAFT ni quedarse.
		{domain.StateActive, domain.StateRenewed, true},
		{domain.StateActive, domain.StateTerminated, true},
		{domain.StateActive, domain.StateDraft, false},
		{domain.StateActive, domain.StateActive, false},

		// Desde RENEWED solo se puede rescindir.
		{domain.StateRenewed, domain.StateTerminated, true},
		{domain.StateRenewed, domain.StateDraft, false},
		{domain.StateRenewed, domain.StateActive, false},
		{domain.StateRenewed, domain.StateRenewed, false},

		// TERMINATED es un estado final: no hay transición posible.
		{domain.StateTerminated, domain.StateDraft, false},
		{domain.StateTerminated, domain.StateActive, false},
		{domain.StateTerminated, domain.StateRenewed, false},
		{domain.StateTerminated, domain.StateTerminated, false},

		// Un estado desconocido (dato corrupto/futuro) cae en el default: siempre false.
		{domain.ContractState("BOGUS"), domain.StateActive, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if got := tc.from.CanTransitionTo(tc.to); got != tc.want {
				t.Fatalf("%s.CanTransitionTo(%s): got %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestErrInvalidStateTransition_TieneUnMensajeDescriptivo(t *testing.T) {
	if domain.ErrInvalidStateTransition == nil || domain.ErrInvalidStateTransition.Error() == "" {
		t.Fatal("ErrInvalidStateTransition debería ser un error no vacío")
	}
}
