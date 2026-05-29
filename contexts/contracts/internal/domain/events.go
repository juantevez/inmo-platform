package domain

import (
	"crypto/rand"
	"fmt"
	"inmo.platform/shared/pkg/ddd"
)

type ContractActivated struct {
	ddd.BaseDomainEvent
	PropertyID string `json:"property_id"`
}

func NewContractActivated(contractID, propertyID string) ContractActivated {
	return ContractActivated{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			contractID,
			"contracts.contract.activated",
		),
		PropertyID: propertyID,
	}
}

func nextUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
