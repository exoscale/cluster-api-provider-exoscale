package service

import (
	"fmt"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

func defaultControlPlaneRules(cpSGID uuid.UUID, apiServerPort int32) []domain.SecurityGroupRule {
	all := "0.0.0.0/0"
	return []domain.SecurityGroupRule{
		{
			Description:   "kubernetes API server",
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     int64(apiServerPort),
			EndPort:       int64(apiServerPort),
			Network:       &all,
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
// ponytail: NodePort range (30000-32767) intentionally not opened by default.
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
	}
}

func desiredControlPlaneRules(cpSGID uuid.UUID, apiServerPort int32, userRules []infrav1alpha1.SecurityGroupRule) ([]domain.SecurityGroupRule, error) {
	return mergeWithUserRules(defaultControlPlaneRules(cpSGID, apiServerPort), cpSGID, userRules)
}

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

// specRuleToDomain resolves the "itself" sentinel in SecurityGroup.SecurityGroup
// to the owning SG's UUID.
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

// rulesEqual excludes ID and Description so drift detection doesn't fire on cosmetic changes.
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
