package service

import (
	"context"
	"errors"
	"fmt"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/infrastructure/exoscale"
	"github.com/go-logr/logr"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

type clusterService struct {
	elasticIPSvc     domain.ElasticIPService
	securityGroupSvc domain.SecurityGroupService
	logger           logr.Logger
}

var _ domain.ClusterService = (*clusterService)(nil)
var errInvalidID = errors.New("invalid id")

func NewClusterService(apiKey, apisecret string, zone egoscale.ZoneName, logger logr.Logger) (domain.ClusterService, error) {
	cloudClient, err := exoscale.NewCloudWithLogger(apiKey, apisecret, zone, logger)
	if err != nil {
		return nil, err
	}

	return &clusterService{
		elasticIPSvc:     NewElasticIPService(cloudClient, logger),
		securityGroupSvc: NewSecurityGroupService(cloudClient, logger),
		logger:           logger,
	}, nil
}

func (s *clusterService) ReconcileCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
	var clusterID uuid.UUID
	if cluster.Status.ID == nil {
		return cluster, fmt.Errorf("cluster: %q has no id", cluster.Name)
	} else {
		id, err := uuid.Parse(*cluster.Status.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse \".status.ClusterID\": %w: %w", errInvalidID, err)
		}
		clusterID = id
	}

	/*
	** Elastic IP
	 */
	var eipID *uuid.UUID
	if cluster.Status.ControlPlaneEndpoint != nil {
		id, err := uuid.Parse(cluster.Status.ControlPlaneEndpoint.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.controlPlaneEndpoint.id", errInvalidID, err)
		}
		eipID = &id
	}

	eip, err := s.elasticIPSvc.UpsertElasticIP(ctx, clusterID, eipID, cluster.Spec.ControlPlaneEndpoint.Port)
	if err != nil {
		return cluster, err
	}
	cluster.Spec.ControlPlaneEndpoint.Host = eip.IP
	cluster.Status.ControlPlaneEndpoint = &infrav1alpha1.APIEndpointStatus{
		APIEndpoint: cluster.Spec.ControlPlaneEndpoint,
		ID:          eip.ID.String(),
		Description: eip.Description,
	}

	/*
	** Security Group
	 */
	var securityGroupControlPlaneID *uuid.UUID
	var securityGroupControlPlaneName = fmt.Sprintf("capi - %s - control plane", clusterID.String())
	if cluster.Status.SecurityGroupControlPlan != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupControlPlan.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.securityGroupControlPlane.id", errInvalidID, err)
		}
		securityGroupControlPlaneID = &id
	}
	securityGroupControlPlane, err := s.securityGroupSvc.UpsertSecurityGroup(ctx, clusterID, securityGroupControlPlaneID, securityGroupControlPlaneName)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert security group for control plane: %w", err)
	}
	cluster.Status.SecurityGroupControlPlan = &infrav1alpha1.SecurityGroupStatus{
		ID:   securityGroupControlPlane.ID.String(),
		Name: securityGroupControlPlane.Name,
	}

	var securityGroupNodeID *uuid.UUID
	var securityGroupNodeName = fmt.Sprintf("capi - %s - node", clusterID.String())

	if cluster.Status.SecurityGroupNode != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupNode.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.SecurityGroupNode.id", errInvalidID, err)
		}
		securityGroupNodeID = &id
	}
	securityGroupNode, err := s.securityGroupSvc.UpsertSecurityGroup(ctx, clusterID, securityGroupNodeID, securityGroupNodeName)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert security group for node: %w", err)
	}
	cluster.Status.SecurityGroupNode = &infrav1alpha1.SecurityGroupStatus{
		ID:   securityGroupNode.ID.String(),
		Name: securityGroupNode.Name,
	}

	/*
	** Security Group Rules
	 */
	desiredCPRules, err := desiredControlPlaneRules(securityGroupControlPlane.ID, cluster.Spec.ControlPlaneEndpoint.Port, cluster.Spec.SecurityGroupControlPlane.Rules)
	if err != nil {
		return cluster, fmt.Errorf("unable to build control plane security group rules: %w", err)
	}
	cpRules, err := s.securityGroupSvc.UpsertSecurityGroupRules(ctx, securityGroupControlPlane.ID, desiredCPRules)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert control plane security group rules: %w", err)
	}
	cluster.Status.SecurityGroupControlPlan.Rules = domainRulesToStatus(cpRules)

	desiredNodeRulesList, err := desiredNodeRules(securityGroupControlPlane.ID, securityGroupNode.ID, cluster.Spec.SecurityGroupNode.Rules)
	if err != nil {
		return cluster, fmt.Errorf("unable to build node security group rules: %w", err)
	}
	nodeRules, err := s.securityGroupSvc.UpsertSecurityGroupRules(ctx, securityGroupNode.ID, desiredNodeRulesList)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert node security group rules: %w", err)
	}
	cluster.Status.SecurityGroupNode.Rules = domainRulesToStatus(nodeRules)

	// TODO: cgeck if I need to set this value to false in case of error in update, 1. provisioned = true, 2. reconcile again with error, 3 do I need to update the provisioned = false ?
	cluster.Status.Initialization.Provisioned = new(true)

	return cluster, nil
}

func (s *clusterService) DeleteCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
	clusterID, err := clusterStatusID(cluster)
	if err != nil {
		return cluster, err
	}
	if clusterID == nil && hasClusterResources(cluster) {
		return cluster, fmt.Errorf("cannot delete cluster resources without cluster ID")
	}

	if clusterID != nil {
		eip, err := s.elasticIPSvc.FindElasticIP(ctx, *clusterID)
		if err != nil && !errors.Is(err, domain.ErrElasticIPNotFound) {
			return cluster, fmt.Errorf("unable to recover elastic IP: %w", err)
		}
		if err == nil {
			cluster.Status.ControlPlaneEndpoint = &infrav1alpha1.APIEndpointStatus{ID: eip.ID.String()}
		}

		controlPlaneName := fmt.Sprintf("capi - %s - control plane", clusterID)
		controlPlaneGroup, err := s.securityGroupSvc.FindSecurityGroup(ctx, controlPlaneName)
		if err != nil && !errors.Is(err, domain.ErrSecurityGroupNotFound) {
			return cluster, fmt.Errorf("unable to recover control-plane security group: %w", err)
		}
		if err == nil {
			cluster.Status.SecurityGroupControlPlan = &infrav1alpha1.SecurityGroupStatus{ID: controlPlaneGroup.ID.String(), Name: controlPlaneGroup.Name}
		}

		nodeName := fmt.Sprintf("capi - %s - node", clusterID)
		nodeGroup, err := s.securityGroupSvc.FindSecurityGroup(ctx, nodeName)
		if err != nil && !errors.Is(err, domain.ErrSecurityGroupNotFound) {
			return cluster, fmt.Errorf("unable to recover node security group: %w", err)
		}
		if err == nil {
			cluster.Status.SecurityGroupNode = &infrav1alpha1.SecurityGroupStatus{ID: nodeGroup.ID.String(), Name: nodeGroup.Name}
		}
	}

	if cluster.Status.ControlPlaneEndpoint != nil {
		id, err := uuid.Parse(cluster.Status.ControlPlaneEndpoint.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.controlPlaneEndpoint.id", errInvalidID, err)
		}
		if err := s.elasticIPSvc.DeleteElasticIP(ctx, id, *clusterID); err != nil {
			return cluster, err
		}
	}

	// Rules must be purged from all security groups before deleting them. Security groups can
	// reference each other in their rules (e.g. group A has a rule pointing to group B, and group B
	// has a rule pointing to group A), creating a circular dependency that prevents deletion of either
	// group until both are fully cleared of their rules.
	var securityGroupControlPlaneID *uuid.UUID
	var securityGroupControlPlaneName string
	if cluster.Status.SecurityGroupControlPlan != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupControlPlan.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.securityGroupControlPlane.id", errInvalidID, err)
		}
		securityGroupControlPlaneID = &id
		securityGroupControlPlaneName = fmt.Sprintf("capi - %s - control plane", clusterID)
		if err := s.securityGroupSvc.PurgeSecurityGroup(ctx, id, securityGroupControlPlaneName); err != nil {
			return cluster, err
		}
	}
	var securityGroupNodeID *uuid.UUID
	var securityGroupNodeName string
	if cluster.Status.SecurityGroupNode != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupNode.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.securityGroupNode.id", errInvalidID, err)
		}
		securityGroupNodeID = &id
		securityGroupNodeName = fmt.Sprintf("capi - %s - node", clusterID)
		if err := s.securityGroupSvc.PurgeSecurityGroup(ctx, id, securityGroupNodeName); err != nil {
			return cluster, err
		}
	}

	if securityGroupControlPlaneID != nil {
		if err := s.securityGroupSvc.DeleteSecurityGroup(ctx, *securityGroupControlPlaneID, securityGroupControlPlaneName); err != nil {
			return cluster, fmt.Errorf("unable to delete security group for control plane: %w", err)
		}
	}
	if securityGroupNodeID != nil {
		if err := s.securityGroupSvc.DeleteSecurityGroup(ctx, *securityGroupNodeID, securityGroupNodeName); err != nil {
			return cluster, fmt.Errorf("unable to delete security group for node: %w", err)
		}
	}

	return cluster, nil
}

func hasClusterResources(cluster infrav1alpha1.ExoscaleCluster) bool {
	return cluster.Status.ControlPlaneEndpoint != nil || cluster.Status.SecurityGroupControlPlan != nil || cluster.Status.SecurityGroupNode != nil
}

func clusterStatusID(cluster infrav1alpha1.ExoscaleCluster) (*uuid.UUID, error) {
	if cluster.Status.ID == nil {
		return nil, nil
	}
	id, err := uuid.Parse(*cluster.Status.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to parse %q: %w: %w", ".status.id", errInvalidID, err)
	}
	return &id, nil
}
