package service

import (
	"fmt"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

// defaultControlPlaneRules returns the default rules required to operate a Kubernetes control plane.
// cpSGID is needed for rules whose source is the control plane security group itself.
// apiServerPort is taken from spec.controlPlaneEndpoint.port so the API server rule stays in sync.
func defaultControlPlaneRules(cpSGID uuid.UUID, apiServerPort int32) []domain.SecurityGroupRule {
	return []domain.SecurityGroupRule{
		{
			Description:   "kubernetes API server",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     int64(apiServerPort),
			EndPort:       int64(apiServerPort),
			Network:       func() *string { v := "0.0.0.0/0"; return &v }(),
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
	}
}

// defaultNodeRules returns the hardcoded rules required to operate Kubernetes worker nodes.
// cpSGID and nodeSGID are needed to express inter-SG traffic sources.
func defaultNodeRules(cpSGID, nodeSGID uuid.UUID) []domain.SecurityGroupRule {
	return []domain.SecurityGroupRule{
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
		// TODO: check if we want to allow these port by default
		// {
		// 	Description:   "NodePort services",
		// 	FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		// 	Protocol:      domain.SecurityGroupRuleProtocolTCP,
		// 	StartPort:     30000,
		// 	EndPort:       32767,
		// 	Network:       &cidrAll,
		// },
	}
}

// desiredControlPlaneRules merges the default control plane rules with user-defined rules from the spec.
// itselfID resolves the "itself" self-reference allowed in user rules.
// apiServerPort is taken from spec.controlPlaneEndpoint.port.
func desiredControlPlaneRules(cpSGID uuid.UUID, apiServerPort int32, userRules []infrav1alpha1.SecurityGroupRule) ([]domain.SecurityGroupRule, error) {
	return mergeWithUserRules(defaultControlPlaneRules(cpSGID, apiServerPort), cpSGID, userRules)
}

// desiredNodeRules merges the default node rules with user-defined rules from the spec.
func desiredNodeRules(cpSGID, nodeSGID uuid.UUID, userRules []infrav1alpha1.SecurityGroupRule) ([]domain.SecurityGroupRule, error) {
	return mergeWithUserRules(defaultNodeRules(cpSGID, nodeSGID), nodeSGID, userRules)
}

func mergeWithUserRules(defaults []domain.SecurityGroupRule, itselfID uuid.UUID, userRules []infrav1alpha1.SecurityGroupRule) ([]domain.SecurityGroupRule, error) {
	rules := make([]domain.SecurityGroupRule, len(defaults), len(defaults)+len(userRules))
	copy(rules, defaults)

	for _, r := range userRules {
		domainRule, err := specRuleToDomain(r, itselfID)
		if err != nil {
			return nil, err
		}
		rules = append(rules, domainRule)
	}

	return rules, nil
}

// specRuleToDomain converts an API spec rule to the domain representation.
// itselfID resolves the "itself" special value, which refers to the owning security group.
func specRuleToDomain(rule infrav1alpha1.SecurityGroupRule, itselfID uuid.UUID) (domain.SecurityGroupRule, error) {
	domainRule := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirection(rule.FlowDirection),
		Protocol:      domain.SecurityGroupRuleProtocol(rule.Protocol),
		StartPort:     rule.StartPort,
		EndPort:       rule.EndPort,
		Network:       rule.Network,
		Description:   rule.Description,
	}

	if rule.SecurityGroup != nil {
		if *rule.SecurityGroup == "itself" {
			domainRule.SecurityGroup = &itselfID
		} else {
			id, err := uuid.Parse(*rule.SecurityGroup)
			if err != nil {
				return domain.SecurityGroupRule{}, fmt.Errorf("invalid security group reference %q: %w", *rule.SecurityGroup, err)
			}
			domainRule.SecurityGroup = &id
		}
	}

	return domainRule, nil
}

// domainRulesToStatus converts domain rules to their API status representation.
func domainRulesToStatus(rules []domain.SecurityGroupRule) []infrav1alpha1.SecurityGroupRuleStatus {
	result := make([]infrav1alpha1.SecurityGroupRuleStatus, 0, len(rules))
	for _, r := range rules {
		result = append(result, domainRuleToStatus(r))
	}
	return result
}

func domainRuleToStatus(rule domain.SecurityGroupRule) infrav1alpha1.SecurityGroupRuleStatus {
	status := infrav1alpha1.SecurityGroupRuleStatus{
		ID: rule.ID.String(),
		SecurityGroupRule: infrav1alpha1.SecurityGroupRule{
			FlowDirection: egoscale.SecurityGroupRuleFlowDirection(rule.FlowDirection),
			Protocol:      egoscale.SecurityGroupRuleProtocol(rule.Protocol),
			StartPort:     rule.StartPort,
			EndPort:       rule.EndPort,
			Network:       rule.Network,
			Description:   rule.Description,
		},
	}

	if rule.SecurityGroup != nil {
		status.SecurityGroupRef = &infrav1alpha1.SecurityGroupResource{
			ID: rule.SecurityGroup.String(),
		}
	}

	return status
}

// rulesEqual returns true when two rules enforce the same firewall policy.
// ID and Description are excluded
func rulesEqual(a, b domain.SecurityGroupRule) bool {
	if a.FlowDirection != b.FlowDirection || a.Protocol != b.Protocol {
		return false
	}
	if a.StartPort != b.StartPort || a.EndPort != b.EndPort {
		return false
	}

	networkEqual := (a.Network == nil && b.Network == nil) ||
		(a.Network != nil && b.Network != nil && *a.Network == *b.Network)
	if !networkEqual {
		return false
	}

	sgEqual := (a.SecurityGroup == nil && b.SecurityGroup == nil) ||
		(a.SecurityGroup != nil && b.SecurityGroup != nil && *a.SecurityGroup == *b.SecurityGroup)
	return sgEqual
}

// diffRules returns which rules must be added and which must be deleted to
// reconcile the existing cloud state toward the desired state.
func diffRules(desired, existing []domain.SecurityGroupRule) (toAdd, toDelete []domain.SecurityGroupRule) {
	for _, d := range desired {
		if !ruleIn(existing, d) {
			toAdd = append(toAdd, d)
		}
	}
	for _, e := range existing {
		if !ruleIn(desired, e) {
			toDelete = append(toDelete, e)
		}
	}
	return
}

func ruleIn(rules []domain.SecurityGroupRule, target domain.SecurityGroupRule) bool {
	for _, r := range rules {
		if rulesEqual(r, target) {
			return true
		}
	}
	return false
}
