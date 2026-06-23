package service

import (
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string { return &s }

var rootSubnet string = "0.0.0.0/0"

func Test_defaultControlPlaneRules(t *testing.T) {
	t.Parallel()

	cpSGID := uuid.New()
	apiServerPort := int32(6443)

	rules := defaultControlPlaneRules(cpSGID, apiServerPort)

	assert.Len(t, rules, 5)
	assert.Equal(t, []domain.SecurityGroupRule{
		{
			Description:   "kubernetes API server",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     int64(apiServerPort),
			EndPort:       int64(apiServerPort),
			Network:       &rootSubnet,
		},
		{
			Description:   "etcd client and peer communication",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     2379,
			EndPort:       2380,
			SecurityGroup: &cpSGID,
		},
		{
			Description:   "kubelet API",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: &cpSGID,
		},
		{
			Description:   "kube-controller-manager",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     10257,
			EndPort:       10257,
			SecurityGroup: &cpSGID,
		},
		{
			Description:   "kube-scheduler",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     10259,
			EndPort:       10259,
			SecurityGroup: &cpSGID,
		},
	}, rules)
}

func Test_defaultNodeRules(t *testing.T) {
	t.Parallel()

	cpSGID := uuid.New()
	nodeSGID := uuid.New()

	rules := defaultNodeRules(cpSGID, nodeSGID)

	assert.Equal(t, []domain.SecurityGroupRule{
		{
			Description:   "Kubelet API from control plane",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: &cpSGID,
		},
		{
			Description:   "Kubelet API node-to-node",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: &nodeSGID,
		},
	}, rules)
}

func Test_specRuleToDomain(t *testing.T) {
	t.Parallel()

	itselfID := uuid.New()
	otherSGID := uuid.New()
	cidr := "10.0.0.0/8"

	tests := []struct {
		name string
		rule infrav1alpha1.SecurityGroupRule
		want domain.SecurityGroupRule
		err  string
	}{
		{
			name: "network-based rule",
			rule: infrav1alpha1.SecurityGroupRule{
				FlowDirection: egoscale.SecurityGroupRuleFlowDirection("ingress"),
				Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
				StartPort:     80,
				EndPort:       80,
				Network:       &cidr,
				Description:   "http",
			},
			want: domain.SecurityGroupRule{
				FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
				Protocol:      domain.SecurityGroupRuleProtocolTCP,
				StartPort:     80,
				EndPort:       80,
				Network:       &cidr,
				Description:   "http",
			},
		},
		{
			name: "itself security group reference",
			rule: infrav1alpha1.SecurityGroupRule{
				FlowDirection: egoscale.SecurityGroupRuleFlowDirection("ingress"),
				Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
				StartPort:     10250,
				EndPort:       10250,
				SecurityGroup: strPtr("itself"),
			},
			want: domain.SecurityGroupRule{
				FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
				Protocol:      domain.SecurityGroupRuleProtocolTCP,
				StartPort:     10250,
				EndPort:       10250,
				SecurityGroup: &itselfID,
			},
		},
		{
			name: "explicit security group UUID reference",
			rule: infrav1alpha1.SecurityGroupRule{
				FlowDirection: egoscale.SecurityGroupRuleFlowDirection("egress"),
				Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
				StartPort:     443,
				EndPort:       443,
				SecurityGroup: strPtr(otherSGID.String()),
			},
			want: domain.SecurityGroupRule{
				FlowDirection: domain.SecurityGroupRuleFlowDirectionEgress,
				Protocol:      domain.SecurityGroupRuleProtocolTCP,
				StartPort:     443,
				EndPort:       443,
				SecurityGroup: &otherSGID,
			},
		},
		{
			name: "invalid security group UUID",
			rule: infrav1alpha1.SecurityGroupRule{SecurityGroup: strPtr("not-a-uuid")},
			err:  "invalid UUID",
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			got, err := specRuleToDomain(ut.rule, itselfID)

			if ut.err != "" {
				assert.ErrorContains(t, err, ut.err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, ut.want, got)
		})
	}
}

func Test_mergeWithUserRules(t *testing.T) {
	t.Parallel()

	defaultRule := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     6443,
		EndPort:       6443,
		Network:       &rootSubnet,
	}
	userRule := infrav1alpha1.SecurityGroupRule{
		FlowDirection: egoscale.SecurityGroupRuleFlowDirection("ingress"),
		Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
		StartPort:     8080,
		EndPort:       8080,
		Network:       &rootSubnet,
		Description:   "extra",
	}
	expectedUserRule := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     8080,
		EndPort:       8080,
		Network:       &rootSubnet,
		Description:   "extra",
	}

	tests := []struct {
		name      string
		defaults  []domain.SecurityGroupRule
		userRules []infrav1alpha1.SecurityGroupRule
		want      []domain.SecurityGroupRule
		err       string
	}{
		{
			name:     "no user rules",
			defaults: []domain.SecurityGroupRule{defaultRule},
			want:     []domain.SecurityGroupRule{defaultRule},
		},
		{
			name:      "user rule appended",
			defaults:  []domain.SecurityGroupRule{defaultRule},
			userRules: []infrav1alpha1.SecurityGroupRule{userRule},
			want:      []domain.SecurityGroupRule{defaultRule, expectedUserRule},
		},
		{
			name:      "invalid user rule returns error",
			defaults:  []domain.SecurityGroupRule{defaultRule},
			userRules: []infrav1alpha1.SecurityGroupRule{{SecurityGroup: strPtr("not-a-uuid")}},
			err:       "invalid UUID",
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			got, err := mergeWithUserRules(ut.defaults, uuid.New(), ut.userRules)

			if ut.err != "" {
				assert.ErrorContains(t, err, ut.err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, ut.want, got)
		})
	}
}

func Test_rulesEqual(t *testing.T) {
	t.Parallel()

	cidr := "10.1.0.0/16"
	otherCIDR := "192.168.0.0/16"
	sgID := uuid.New()
	otherID := uuid.New()

	// base uses a network source; baseSG uses a security group source.
	base := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
		Network:       &cidr,
	}
	baseSG := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
		SecurityGroup: &sgID,
	}

	tests := []struct {
		name  string
		a, b  domain.SecurityGroupRule
		equal bool
	}{
		{
			name:  "identical rules with network",
			a:     base,
			b:     base,
			equal: true,
		},
		{
			name:  "identical rules with security group",
			a:     baseSG,
			b:     baseSG,
			equal: true,
		},
		{
			name: "different flow direction",
			a:    base,
			b: func() domain.SecurityGroupRule {
				r := base
				r.FlowDirection = domain.SecurityGroupRuleFlowDirectionEgress
				return r
			}(),
			equal: false,
		},
		{
			name:  "different protocol",
			a:     base,
			b:     func() domain.SecurityGroupRule { r := base; r.Protocol = domain.SecurityGroupRuleProtocolUDP; return r }(),
			equal: false,
		},
		{
			name:  "different start port",
			a:     base,
			b:     func() domain.SecurityGroupRule { r := base; r.StartPort = 443; return r }(),
			equal: false,
		},
		{
			name:  "different end port",
			a:     base,
			b:     func() domain.SecurityGroupRule { r := base; r.EndPort = 443; return r }(),
			equal: false,
		},
		{
			name:  "different networks",
			a:     base,
			b:     func() domain.SecurityGroupRule { r := base; r.Network = &otherCIDR; return r }(),
			equal: false,
		},
		{
			name:  "network vs security group",
			a:     base,
			b:     baseSG,
			equal: false,
		},
		{
			name:  "different security groups",
			a:     baseSG,
			b:     func() domain.SecurityGroupRule { r := baseSG; r.SecurityGroup = &otherID; return r }(),
			equal: false,
		},
		{
			name:  "ID and Description are ignored",
			a:     func() domain.SecurityGroupRule { r := base; r.ID = uuid.New(); r.Description = "foo"; return r }(),
			b:     func() domain.SecurityGroupRule { r := base; r.ID = uuid.New(); r.Description = "bar"; return r }(),
			equal: true,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			assert.Equal(t, ut.equal, rulesEqual(ut.a, ut.b))
		})
	}
}

func Test_diffRules(t *testing.T) {
	t.Parallel()

	rule80 := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
		Network:       &rootSubnet,
	}
	rule443 := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     443,
		EndPort:       443,
		Network:       &rootSubnet,
	}
	rule8080 := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     8080,
		EndPort:       8080,
		Network:       &rootSubnet,
	}

	tests := []struct {
		name     string
		desired  []domain.SecurityGroupRule
		existing []domain.SecurityGroupRule
		wantAdd  []domain.SecurityGroupRule
		wantDel  []domain.SecurityGroupRule
	}{
		{
			name:     "no changes",
			desired:  []domain.SecurityGroupRule{rule80},
			existing: []domain.SecurityGroupRule{rule80},
		},
		{
			name:     "add new rule",
			desired:  []domain.SecurityGroupRule{rule80, rule443},
			existing: []domain.SecurityGroupRule{rule80},
			wantAdd:  []domain.SecurityGroupRule{rule443},
		},
		{
			name:     "delete removed rule",
			desired:  []domain.SecurityGroupRule{rule80},
			existing: []domain.SecurityGroupRule{rule80, rule443},
			wantDel:  []domain.SecurityGroupRule{rule443},
		},
		{
			name:     "add and delete simultaneously",
			desired:  []domain.SecurityGroupRule{rule80, rule8080},
			existing: []domain.SecurityGroupRule{rule80, rule443},
			wantAdd:  []domain.SecurityGroupRule{rule8080},
			wantDel:  []domain.SecurityGroupRule{rule443},
		},
		{
			name:     "empty desired deletes all",
			existing: []domain.SecurityGroupRule{rule80, rule443},
			wantDel:  []domain.SecurityGroupRule{rule80, rule443},
		},
		{
			name:    "empty existing adds all",
			desired: []domain.SecurityGroupRule{rule80, rule443},
			wantAdd: []domain.SecurityGroupRule{rule80, rule443},
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			toAdd, toDel := diffRules(ut.desired, ut.existing)
			assert.Equal(t, ut.wantAdd, toAdd)
			assert.Equal(t, ut.wantDel, toDel)
		})
	}
}

func Test_ruleIn(t *testing.T) {
	t.Parallel()

	rule80 := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
		Network:       &rootSubnet,
	}
	rule443 := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     443,
		EndPort:       443,
		Network:       &rootSubnet,
	}

	assert.True(t, ruleIn([]domain.SecurityGroupRule{rule80, rule443}, rule80))
	assert.False(t, ruleIn([]domain.SecurityGroupRule{rule443}, rule80))
	assert.False(t, ruleIn(nil, rule80))
}

func Test_domainRuleToStatus(t *testing.T) {
	t.Parallel()

	sgID := uuid.New()
	ruleID := uuid.New()
	cidr := "10.2.0.0/16"

	tests := []struct {
		name string
		rule domain.SecurityGroupRule
		want infrav1alpha1.SecurityGroupRuleStatus
	}{
		{
			name: "rule with network",
			rule: domain.SecurityGroupRule{
				ID:            ruleID,
				FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
				Protocol:      domain.SecurityGroupRuleProtocolTCP,
				StartPort:     80,
				EndPort:       80,
				Network:       &cidr,
				Description:   "http",
			},
			want: infrav1alpha1.SecurityGroupRuleStatus{
				ID: ruleID.String(),
				SecurityGroupRule: infrav1alpha1.SecurityGroupRule{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirection("ingress"),
					Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
					StartPort:     80,
					EndPort:       80,
					Network:       &cidr,
					Description:   "http",
				},
			},
		},
		{
			name: "rule with security group ref",
			rule: domain.SecurityGroupRule{
				ID:            ruleID,
				FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
				Protocol:      domain.SecurityGroupRuleProtocolTCP,
				StartPort:     10250,
				EndPort:       10250,
				SecurityGroup: &sgID,
			},
			want: infrav1alpha1.SecurityGroupRuleStatus{
				ID: ruleID.String(),
				SecurityGroupRule: infrav1alpha1.SecurityGroupRule{
					FlowDirection: egoscale.SecurityGroupRuleFlowDirection("ingress"),
					Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
					StartPort:     10250,
					EndPort:       10250,
				},
				SecurityGroupRef: &infrav1alpha1.SecurityGroupResource{
					ID: sgID.String(),
				},
			},
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			assert.Equal(t, ut.want, domainRuleToStatus(ut.rule))
		})
	}
}

func Test_domainRulesToStatus(t *testing.T) {
	t.Parallel()

	sgID := uuid.New()
	cidr := "10.3.0.0/16"

	rules := []domain.SecurityGroupRule{
		{
			ID:            uuid.New(),
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     80,
			EndPort:       80,
			Network:       &cidr,
		},
		{
			ID:            uuid.New(),
			FlowDirection: domain.SecurityGroupRuleFlowDirectionEgress,
			Protocol:      domain.SecurityGroupRuleProtocolUDP,
			StartPort:     53,
			EndPort:       53,
			SecurityGroup: &sgID,
		},
	}

	assert.Equal(t, []infrav1alpha1.SecurityGroupRuleStatus{
		{
			ID: rules[0].ID.String(),
			SecurityGroupRule: infrav1alpha1.SecurityGroupRule{
				FlowDirection: egoscale.SecurityGroupRuleFlowDirection("ingress"),
				Protocol:      egoscale.SecurityGroupRuleProtocol("tcp"),
				StartPort:     80,
				EndPort:       80,
				Network:       &cidr,
			},
		},
		{
			ID: rules[1].ID.String(),
			SecurityGroupRule: infrav1alpha1.SecurityGroupRule{
				FlowDirection: egoscale.SecurityGroupRuleFlowDirection("egress"),
				Protocol:      egoscale.SecurityGroupRuleProtocol("udp"),
				StartPort:     53,
				EndPort:       53,
			},
			SecurityGroupRef: &infrav1alpha1.SecurityGroupResource{
				ID: sgID.String(),
			},
		},
	}, domainRulesToStatus(rules))
}
