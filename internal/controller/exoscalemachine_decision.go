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

type machineAction string

const (
	machineActionUpsert           machineAction = "upsert"
	machineActionWait             machineAction = "wait"
	machineActionReady            machineAction = "ready"
	machineActionDelete           machineAction = "delete"
	machineActionCompleteDeletion machineAction = "complete-deletion"
)

type machineDecision struct {
	action  machineAction
	upsert  upsertInstanceAction
	outcome machineOutcome
}

type upsertInstanceAction struct {
	machineID  domain.MachineID
	instanceID *uuid.UUID
	spec       domain.InstanceSpec
}

type machineOutcome struct {
	instanceID    string
	instanceState string
	providerID    string
	addresses     []machineAddress
	message       string
	requeueAfter  time.Duration
}

type machineAddress struct {
	public  bool
	address string
}

func decideMachine(input machineDecisionInput) machineDecision {
	if input.upsertResult == nil {
		return machineDecision{action: machineActionUpsert, upsert: upsertInstanceAction{
			machineID:  input.machineID,
			instanceID: input.instanceID,
			spec:       input.spec,
		}}
	}

	instance := input.upsertResult
	outcome := machineOutcome{
		instanceID:    instance.ID.String(),
		instanceState: instance.State,
	}
	if instance.State != "running" {
		outcome.message = fmt.Sprintf("Instance state is %s", instance.State)
		outcome.requeueAfter = 15 * time.Second
		return machineDecision{action: machineActionWait, outcome: outcome}
	}

	outcome.providerID = fmt.Sprintf("exoscale://%s", instance.ID)
	outcome.message = "Instance is running"
	if instance.PublicIP != "" {
		outcome.addresses = append(outcome.addresses, machineAddress{public: true, address: instance.PublicIP})
	}
	if instance.PrivateIP != "" {
		outcome.addresses = append(outcome.addresses, machineAddress{address: instance.PrivateIP})
	}

	return machineDecision{action: machineActionReady, outcome: outcome}
}
