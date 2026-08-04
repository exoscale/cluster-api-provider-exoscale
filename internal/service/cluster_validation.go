package service

import (
	"fmt"
	"slices"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/util/validation/field"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
)

var validZones = []egoscale.ZoneName{
	egoscale.ZoneNameATVie1,
	egoscale.ZoneNameATVie2,
	egoscale.ZoneNameBGSof1,
	egoscale.ZoneNameCHDk2,
	egoscale.ZoneNameCHGva2,
	egoscale.ZoneNameDEFra1,
	egoscale.ZoneNameDEMuc1,
	egoscale.ZoneNameHrZag1,
}

var validFlowDirections = []egoscale.SecurityGroupRuleFlowDirection{
	egoscale.SecurityGroupRuleFlowDirectionIngress,
	egoscale.SecurityGroupRuleFlowDirectionEgress,
}

var validProtocols = []egoscale.SecurityGroupRuleProtocol{
	egoscale.SecurityGroupRuleProtocolTCP,
	egoscale.SecurityGroupRuleProtocolUDP,
	egoscale.SecurityGroupRuleProtocolICMP,
	egoscale.SecurityGroupRuleProtocolIcmpv6,
	egoscale.SecurityGroupRuleProtocolEsp,
	egoscale.SecurityGroupRuleProtocolAh,
	egoscale.SecurityGroupRuleProtocolGre,
	egoscale.SecurityGroupRuleProtocolIpip,
}

// exoscaleClusterValidator implements domain.ClusterValidator.
type exoscaleClusterValidator struct{}

var _ domain.ClusterValidator = (*exoscaleClusterValidator)(nil)

func NewExoscaleClusterValidator() domain.ClusterValidator {
	return &exoscaleClusterValidator{}
}

// ValidateCreate validates an ExoscaleClusterSpec on creation.
func (v *exoscaleClusterValidator) ValidateCreate(spec infrav1alpha1.ExoscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.ControlPlaneEndpoint.Host != "" {
		allErrs = append(allErrs, field.Forbidden(
			fldPath.Child("controlPlaneEndpoint", "host"),
			"must be empty; it is set automatically by the controller once the control plane is provisioned",
		))
	}

	allErrs = append(allErrs, v.validate(spec, fldPath)...)

	return allErrs
}

// ValidateUpdate validates an ExoscaleClusterSpec on update.
// NOTE: oldSpec is currently unused. Enforcing that only the controller (and not a user)
// may set or change spec.controlPlaneEndpoint.host is intentionally left for later.
func (v *exoscaleClusterValidator) ValidateUpdate(_, newSpec infrav1alpha1.ExoscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	return v.validate(newSpec, fldPath)
}

func (v *exoscaleClusterValidator) validate(spec infrav1alpha1.ExoscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !slices.Contains(validZones, spec.Zone) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("zone"), spec.Zone, validZones))
	}

	allErrs = append(allErrs, v.validateSecurityGroup(
		spec.SecurityGroupControlPlane,
		defaultControlPlaneRules(spec.ControlPlaneEndpoint.Port),
		fldPath.Child("securityGroupControlPlane"),
	)...)

	allErrs = append(allErrs, v.validateSecurityGroup(
		spec.SecurityGroupWorker,
		defaultWorkerRules(),
		fldPath.Child("securityGroupWorker"),
	)...)

	return allErrs
}

// validateSecurityGroup validates the user-defined rules of a security group, and rejects
// any rule whose name (description) collides with one of the default rules the controller
// adds automatically.
func (v *exoscaleClusterValidator) validateSecurityGroup(sg infrav1alpha1.SecurityGroup, defaults []infrav1alpha1.SecurityGroupRule, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	reservedNames := make(map[string]struct{}, len(defaults))
	for _, d := range defaults {
		reservedNames[d.Description] = struct{}{}
	}

	rulesPath := fldPath.Child("rules")
	for i, rule := range sg.Rules {
		rulePath := rulesPath.Index(i)

		if !slices.Contains(validFlowDirections, rule.FlowDirection) {
			allErrs = append(allErrs, field.NotSupported(fldPath.Child("flowDirection"), rule.FlowDirection, validFlowDirections))
		}

		if !slices.Contains(validProtocols, rule.Protocol) {
			allErrs = append(allErrs, field.NotSupported(fldPath.Child("protocol"), rule.Protocol, validProtocols))
		}

		if rule.StartPort > rule.EndPort {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("endPort"), rule.EndPort, "must be greater than or equal to startPort"))
		}

		switch {
		case rule.Network == nil && rule.SecurityGroup == nil:
			allErrs = append(allErrs, field.Required(fldPath, "exactly one of network or securityGroup must be set"))
		case rule.Network != nil && rule.SecurityGroup != nil:
			allErrs = append(allErrs, field.Invalid(fldPath, rule, "only one of network or securityGroup may be set"))
		case rule.SecurityGroup != nil:
			allErrs = append(allErrs, v.validateSecurityGroupRef(*rule.SecurityGroup, fldPath.Child("securityGroup"))...)
		}

		if _, ok := reservedNames[rule.Description]; ok {
			allErrs = append(allErrs, field.Invalid(
				rulePath.Child("description"), rule.Description,
				"conflicts with the name of a default rule; choose a different description",
			))
		}
	}

	return allErrs
}

// ref must be a uuid or the keyword `worker`/`control-plane`
func (v *exoscaleClusterValidator) validateSecurityGroupRef(ref string, fldPath *field.Path) field.ErrorList {
	switch securityGroupRef(ref) {
	case securityGroupRefControlPlane, securityGroupRefWorker:
		return nil
	}

	if _, err := uuid.Parse(ref); err != nil {
		return field.ErrorList{field.Invalid(
			fldPath, ref,
			fmt.Sprintf("must be a valid UUID, %q, or %q", securityGroupRefControlPlane, securityGroupRefWorker),
		)}
	}

	return nil
}
