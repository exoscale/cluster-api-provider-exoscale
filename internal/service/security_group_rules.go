package service

import (
	"fmt"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

// securityGroupRef is used to reference a security group in a rule. A rule can
// reference the uuid of a security group, a network, or this type. This type
// exists because the user cannot know the uuid of the to-be-created security
// group.
type securityGroupRef string

const (
	securityGroupRefControlPlane securityGroupRef = "control-plane"
	securityGroupRefWorker       securityGroupRef = "worker"
)

// defaultControlPlaneRules returns the default rules required to operate a Kubernetes control plane.
// apiServerPort is taken from spec.controlPlaneEndpoint.port so the API server rule stays in sync.
func defaultControlPlaneRules(apiServerPort int32) []infrav1alpha1.SecurityGroupRule {
	return []infrav1alpha1.SecurityGroupRule{
		{
			Description:   "kubernetes API server",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     int64(apiServerPort),
			EndPort:       int64(apiServerPort),
			Network:       new("0.0.0.0/0"),
		},
		{
			Description:   "etcd client and peer communication",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     2379,
			EndPort:       2380,
			SecurityGroup: new(string(securityGroupRefControlPlane)),
		},
		{
			Description:   "kubelet API",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: new(string(securityGroupRefControlPlane)),
		},
		{
			Description:   "kube-controller-manager",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10257,
			EndPort:       10257,
			SecurityGroup: new(string(securityGroupRefControlPlane)),
		},
		{
			Description:   "kube-scheduler",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10259,
			EndPort:       10259,
			SecurityGroup: new(string(securityGroupRefControlPlane)),
		},
	}
}

// defaultWorkerRules returns the default rules required to operate Kubernetes workers.
func defaultWorkerRules() []infrav1alpha1.SecurityGroupRule {
	return []infrav1alpha1.SecurityGroupRule{
		{
			Description:   "Kubelet API from control plane",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: new(string(securityGroupRefControlPlane)),
		},
		{
			Description:   "Kubelet API worker-to-worker",
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: new(string(securityGroupRefWorker)),
		},
		// TODO: check if we want to allow these port by default
		// {
		// 	Description:   "NodePort services",
		// 	FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
		// 	Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
		// 	StartPort:     30000,
		// 	EndPort:       32767,
		// 	Network:       &cidrAll,
		// },
	}
}

// resolveRules concatenates default and user-defined spec rules and resolves them all, in one pass,
// to their domain representation via specRuleToDomain. cpSGID/workerSGID resolve the
// "control-plane"/"worker" special values that either list may use.
func resolveRules(cpSGID, workerSGID uuid.UUID, defaults, userRules []infrav1alpha1.SecurityGroupRule) ([]domain.SecurityGroupRule, error) {
	all := make([]infrav1alpha1.SecurityGroupRule, 0, len(defaults)+len(userRules))
	all = append(all, defaults...)
	all = append(all, userRules...)

	resolved := make([]domain.SecurityGroupRule, 0, len(all))
	for _, r := range all {
		domainRule, err := specRuleToDomain(r, cpSGID, workerSGID)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, domainRule)
	}

	return resolved, nil
}

// specRuleToDomain converts an API spec rule to the domain representation.
// cpSGID/workerSGID resolve the "control-plane"/"worker" special values, which refer to
// this cluster's managed control-plane/worker security group.
func specRuleToDomain(rule infrav1alpha1.SecurityGroupRule, cpSGID, workerSGID uuid.UUID) (domain.SecurityGroupRule, error) {
	domainRule := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirection(rule.FlowDirection),
		Protocol:      domain.SecurityGroupRuleProtocol(rule.Protocol),
		StartPort:     rule.StartPort,
		EndPort:       rule.EndPort,
		Network:       rule.Network,
		Description:   rule.Description,
	}

	if rule.SecurityGroup != nil {
		switch securityGroupRef(*rule.SecurityGroup) {
		case securityGroupRefControlPlane:
			domainRule.SecurityGroup = &cpSGID
		case securityGroupRefWorker:
			domainRule.SecurityGroup = &workerSGID
		default:
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
