package controller

import (
	"fmt"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/google/uuid"
)

type machineDecisionInput struct {
	machineID    domain.MachineID
	instanceID   *uuid.UUID
	spec         domain.InstanceSpec
	upsertResult *domain.Instance
}

type machineDecision struct {
	upsert  *upsertInstanceAction
	outcome *machineOutcome
}

type upsertInstanceAction struct {
	machineID  domain.MachineID
	instanceID *uuid.UUID
	spec       domain.InstanceSpec
}

type machineOutcome struct {
	instanceID       string
	instanceState    string
	providerID       *string
	provisioned      *bool
	addresses        []machineAddress
	replaceAddresses bool
	ready            bool
	message          string
	requeueAfter     time.Duration
}

type machineAddress struct {
	public  bool
	address string
}

func decideMachine(input machineDecisionInput) machineDecision {
	if input.upsertResult == nil {
		return machineDecision{upsert: &upsertInstanceAction{
			machineID:  input.machineID,
			instanceID: input.instanceID,
			spec:       input.spec,
		}}
	}

	instance := input.upsertResult
	outcome := &machineOutcome{
		instanceID:    instance.ID.String(),
		instanceState: instance.State,
		message:       fmt.Sprintf("Instance state is %s", instance.State),
		requeueAfter:  15 * time.Second,
	}
	if instance.State != "running" {
		return machineDecision{outcome: outcome}
	}

	providerID := fmt.Sprintf("exoscale://%s", instance.ID)
	provisioned := true
	outcome.providerID = &providerID
	outcome.provisioned = &provisioned
	outcome.replaceAddresses = true
	outcome.ready = true
	outcome.message = "Instance is running"
	outcome.requeueAfter = 0
	if instance.PublicIP != "" {
		outcome.addresses = append(outcome.addresses, machineAddress{public: true, address: instance.PublicIP})
	}
	if instance.PrivateIP != "" {
		outcome.addresses = append(outcome.addresses, machineAddress{address: instance.PrivateIP})
	}

	return machineDecision{outcome: outcome}
}
