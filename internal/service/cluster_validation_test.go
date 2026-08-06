package service

import (
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func Test_exoscaleClusterValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input infrav1alpha1.ExoscaleClusterSpec
		err   string
	}{
		{
			name: "valid minimal spec",
			input: infrav1alpha1.ExoscaleClusterSpec{
				Zone:                 egoscale.ZoneNameCHGva2,
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: 6443},
			},
		},
		{
			name: "host must be empty",
			input: infrav1alpha1.ExoscaleClusterSpec{
				Zone: egoscale.ZoneNameCHGva2,
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
					Port: 6443,
					Host: "10.0.0.1",
				},
			},

			err: "must be empty",
		},
		{
			name: "unknown zone",
			input: infrav1alpha1.ExoscaleClusterSpec{
				Zone:                 egoscale.ZoneName("xx-xxx-1"),
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: 6443},
			},
			err: `Unsupported value: "xx-xxx-1"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			errs := NewExoscaleClusterValidator().ValidateCreate(tc.input, field.NewPath("spec"))

			if tc.err == "" {
				assert.Empty(t, errs)
				return
			}

			assert.NotEmpty(t, errs)
			assert.ErrorContains(t, errs.ToAggregate(), tc.err)
		})
	}
}

func Test_exoscaleClusterValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		oldSpec infrav1alpha1.ExoscaleClusterSpec
		newSpec infrav1alpha1.ExoscaleClusterSpec
		err     string
	}{
		{
			name: "zone unchanged is valid",
			oldSpec: infrav1alpha1.ExoscaleClusterSpec{
				Zone:                 egoscale.ZoneNameCHGva2,
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: 6443},
			},
			newSpec: infrav1alpha1.ExoscaleClusterSpec{
				Zone:                 egoscale.ZoneNameCHGva2,
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: 6443},
			},
		},
		{
			name: "zone changed is forbidden",
			oldSpec: infrav1alpha1.ExoscaleClusterSpec{
				Zone:                 egoscale.ZoneNameCHGva2,
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: 6443},
			},
			newSpec: infrav1alpha1.ExoscaleClusterSpec{
				Zone:                 egoscale.ZoneNameDEFra1,
				ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: 6443},
			},
			err: "zone cannot be changed once set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := NewExoscaleClusterValidator().ValidateUpdate(tc.oldSpec, tc.newSpec, field.NewPath("spec"))

			if tc.err == "" {
				assert.Empty(t, errs)
				return
			}

			assert.NotEmpty(t, errs)
			assert.ErrorContains(t, errs.ToAggregate(), tc.err)
		})
	}
}

func Test_exoscaleClusterValidator_validateSecurityGroup(t *testing.T) {
	t.Parallel()

	validSGID := uuid.New().String()

	// fakeDefaultRules := []infrav1alpha1.SecurityGroupRule{
	// 	{
	// 		FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
	// 		Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
	// 		StartPort:     6443, EndPort: 6443,
	// 		Description: "kubernetes API server",
	// 		Network:     new("0.0.0.0/0"),
	// 	},
	// }

	tests := []struct {
		name     string
		rules    []infrav1alpha1.SecurityGroupRule
		defaults []infrav1alpha1.SecurityGroupRule
		wantErr  string
	}{
		{
			name: "valid rule with network",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     22, EndPort: 22,
					Description: "ssh",
					Network:     new("0.0.0.0/0"),
				},
			},
		},
		{
			name: "rule with invalid network",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     22, EndPort: 22,
					Description: "ssh",
					Network:     new("999.999.999.999/32"),
				},
			},
			wantErr: "must be a valid CIDR address",
		},
		{
			name: "rule missing network and securityGroup",
			rules: []infrav1alpha1.SecurityGroupRule{
				{FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress, Protocol: egoscale.SecurityGroupRuleProtocolTCP, StartPort: 80, EndPort: 80, Description: "custom"},
			},
			wantErr: "exactly one of network or securityGroup must be set",
		},
		{
			name: "rule with both network and securityGroup",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					Network:       new("0.0.0.0/0"),
					SecurityGroup: &validSGID,
				},
			},
			wantErr: "only one of network or securityGroup may be set",
		},
		{
			name: "rule with invalid flow direction",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirection("sideways"),
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					Network:       new("0.0.0.0/0"),
				},
			},
			wantErr: `Unsupported value: "sideways"`,
		},
		{
			name: "rule with invalid protocol",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocol("carrier-pigeon"),
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					Network:       new("0.0.0.0/0"),
				},
			},
			wantErr: `Unsupported value: "carrier-pigeon"`,
		},
		{
			name: "rule with startPort after endPort",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     100,
					EndPort:       80,
					Description:   "custom",
					Network:       new("0.0.0.0/0"),
				},
			},
			wantErr: "must be greater than or equal to startPort",
		},
		{
			name: "rule securityGroup control-plane alias is valid",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					SecurityGroup: new("control-plane"),
				},
			},
		},
		{
			name: "rule securityGroup worker alias is valid",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					SecurityGroup: new("worker"),
				},
			},
		},
		{
			name: "rule securityGroup uuid is valid",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					SecurityGroup: new(validSGID),
				},
			},
		},
		{
			name: "rule securityGroup neither uuid nor alias",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Description:   "custom",
					SecurityGroup: new("not-a-uuid"),
				},
			},
			wantErr: `must be a valid UUID, "control-plane", or "worker"`,
		},
		{
			name: "rule name conflicts with a default rule",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     8080,
					EndPort:       8080,
					Description:   "kubernetes API server",
					Network:       new("0.0.0.0/0"),
				},
			},
			defaults: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     6443, EndPort: 6443,
					Description: "kubernetes API server",
					Network:     new("0.0.0.0/0"),
				},
			},
			wantErr: "conflicts with the name of a default rule",
		},
		{
			name: "rule name does not conflict with a default rule from another security group",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     8080, EndPort: 8080,
					Description: "Kubelet API from control plane",
					Network:     new("0.0.0.0/0"),
				},
			},
			defaults: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     6443, EndPort: 6443,
					Description: "kubernetes API server",
					Network:     new("0.0.0.0/0"),
				},
			},
		},
		{
			name: "rule policy duplicates another rule's policy despite a different description",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80, EndPort: 80,
					Description: "first",
					Network:     new("0.0.0.0/0"),
				},
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     80, EndPort: 80,
					Description: "second",
					Network:     new("0.0.0.0/0"),
				},
			},
			wantErr: `duplicates the policy of rule`,
		},
		{
			name: "rule policy duplicates a default rule's policy despite a different description",
			rules: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     6443, EndPort: 6443,
					Description: "custom API server rule",
					Network:     new("0.0.0.0/0"),
				},
			},
			defaults: []infrav1alpha1.SecurityGroupRule{
				{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
					StartPort:     6443, EndPort: 6443,
					Description: "kubernetes API server",
					Network:     new("0.0.0.0/0"),
				},
			},
			wantErr: `duplicates the policy of rule "kubernetes API server"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &exoscaleClusterValidator{}
			sg := infrav1alpha1.SecurityGroup{Rules: tc.rules}

			errs := v.validateSecurityGroup(sg, tc.defaults, field.NewPath("spec", "securityGroup"))

			if tc.wantErr == "" {
				assert.Empty(t, errs)
				return
			}

			assert.NotEmpty(t, errs)
			assert.ErrorContains(t, errs.ToAggregate(), tc.wantErr)
		})
	}
}
