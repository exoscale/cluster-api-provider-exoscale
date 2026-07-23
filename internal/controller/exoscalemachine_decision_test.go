package controller

import (
	"testing"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestDecideMachine(t *testing.T) {
	t.Parallel()

	machineID := domain.MachineID("machine-uid")
	currentInstanceID := uuid.New()
	resultInstanceID := uuid.New()
	spec := domain.InstanceSpec{Name: "machine", Template: "template", InstanceType: "standard.small"}
	providerID := "exoscale://" + resultInstanceID.String()
	pending := func(instance domain.Instance) machineDecision {
		return machineDecision{action: machineActionWait, outcome: machineOutcome{
			instanceID:    instance.ID.String(),
			instanceState: instance.State,
			message:       "Instance state is " + instance.State,
			requeueAfter:  15 * time.Second,
		}}
	}

	tests := []struct {
		name  string
		input machineDecisionInput
		want  machineDecision
	}{
		{
			name: "no result returns upsert action",
			input: machineDecisionInput{
				machineID:  machineID,
				instanceID: &currentInstanceID,
				spec:       spec,
			},
			want: machineDecision{action: machineActionUpsert, upsert: upsertInstanceAction{
				machineID:  machineID,
				instanceID: &currentInstanceID,
				spec:       spec,
			}},
		},
		{
			name:  "starting is pending",
			input: machineDecisionInput{upsertResult: &domain.Instance{ID: resultInstanceID, State: "starting"}},
			want:  pending(domain.Instance{ID: resultInstanceID, State: "starting"}),
		},
		{
			name:  "capitalized Running is pending",
			input: machineDecisionInput{upsertResult: &domain.Instance{ID: resultInstanceID, State: "Running"}},
			want:  pending(domain.Instance{ID: resultInstanceID, State: "Running"}),
		},
		{
			name: "running with both addresses is ready",
			input: machineDecisionInput{upsertResult: &domain.Instance{
				ID: resultInstanceID, State: "running", PublicIP: "1.2.3.4", PrivateIP: "10.0.0.1",
			}},
			want: machineDecision{action: machineActionReady, outcome: machineOutcome{
				instanceID:    resultInstanceID.String(),
				instanceState: "running",
				providerID:    providerID,
				addresses:     []machineAddress{{public: true, address: "1.2.3.4"}, {address: "10.0.0.1"}},
				message:       "Instance is running",
			}},
		},
		{
			name:  "running without addresses replaces them with empty",
			input: machineDecisionInput{upsertResult: &domain.Instance{ID: resultInstanceID, State: "running"}},
			want: machineDecision{action: machineActionReady, outcome: machineOutcome{
				instanceID:    resultInstanceID.String(),
				instanceState: "running",
				providerID:    providerID,
				message:       "Instance is running",
			}},
		},
		{
			name:  "zero-valued result is pending",
			input: machineDecisionInput{upsertResult: &domain.Instance{}},
			want:  pending(domain.Instance{}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, decideMachine(tc.input))
		})
	}
}
