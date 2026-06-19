package service

import (
	"context"
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

func NewClusterService(apiKey, apisecret string, zone egoscale.ZoneName, logger logr.Logger) (*clusterService, error) {
	cloudClient, err := exoscale.NewClient(apiKey, apisecret, zone)
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
	if cluster.Status.ID == nil {
		return cluster, fmt.Errorf("cluster %q has no status.id", cluster.Name)
	}
	clusterID, err := uuid.Parse(*cluster.Status.ID)
	if err != nil {
		return cluster, fmt.Errorf("unable to parse status.id: %w", err)
	}

	var eipID *uuid.UUID
	if cluster.Status.ControlPlaneEndpoint != nil && cluster.Status.ControlPlaneEndpoint.ID != "" {
		id, err := uuid.Parse(cluster.Status.ControlPlaneEndpoint.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse status.controlPlaneEndpoint.id: %w", err)
		}
		eipID = &id
	}

	eip, err := s.elasticIPSvc.UpsertElasticIP(ctx, clusterID, eipID, cluster.Spec.ControlPlaneEndpoint.Port)
	if err != nil {
		return cluster, err
	}

	endpoint := cluster.Spec.ControlPlaneEndpoint
	endpoint.Host = eip.IP
	cluster.Spec.ControlPlaneEndpoint = endpoint
	cluster.Status.ControlPlaneEndpoint = &infrav1alpha1.APIEndpointStatus{
		APIEndpoint: endpoint,
		ID:          eip.ID.String(),
		Description: eip.Description,
	}

	var securityGroupControlPlaneID *uuid.UUID
	securityGroupControlPlaneName := fmt.Sprintf("capi - %s - control plane", clusterID)
	if cluster.Status.SecurityGroupControlPlane != nil && cluster.Status.SecurityGroupControlPlane.ID != "" {
		id, err := uuid.Parse(cluster.Status.SecurityGroupControlPlane.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse status.securityGroupControlPlane.id: %w", err)
		}
		securityGroupControlPlaneID = &id
	}
	securityGroupControlPlane, err := s.securityGroupSvc.UpsertSecurityGroup(ctx, clusterID, securityGroupControlPlaneID, securityGroupControlPlaneName)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert control plane security group: %w", err)
	}
	cluster.Status.SecurityGroupControlPlane = &infrav1alpha1.SecurityGroupStatus{
		ID:   securityGroupControlPlane.ID.String(),
		Name: securityGroupControlPlane.Name,
	}

	var securityGroupNodeID *uuid.UUID
	securityGroupNodeName := fmt.Sprintf("capi - %s - node", clusterID)
	if cluster.Status.SecurityGroupNode != nil && cluster.Status.SecurityGroupNode.ID != "" {
		id, err := uuid.Parse(cluster.Status.SecurityGroupNode.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse status.securityGroupNode.id: %w", err)
		}
		securityGroupNodeID = &id
	}
	securityGroupNode, err := s.securityGroupSvc.UpsertSecurityGroup(ctx, clusterID, securityGroupNodeID, securityGroupNodeName)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert node security group: %w", err)
	}
	cluster.Status.SecurityGroupNode = &infrav1alpha1.SecurityGroupStatus{
		ID:   securityGroupNode.ID.String(),
		Name: securityGroupNode.Name,
	}

	desiredCPRules, err := desiredControlPlaneRules(securityGroupControlPlane.ID, cluster.Spec.ControlPlaneEndpoint.Port, cluster.Spec.SecurityGroupControlPlane.Rules)
	if err != nil {
		return cluster, fmt.Errorf("unable to build control plane security group rules: %w", err)
	}
	cpRules, err := s.securityGroupSvc.UpsertSecurityGroupRules(ctx, securityGroupControlPlane.ID, desiredCPRules)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert control plane security group rules: %w", err)
	}
	cluster.Status.SecurityGroupControlPlane.Rules = domainRulesToStatus(cpRules)

	desiredNodeRulesList, err := desiredNodeRules(securityGroupControlPlane.ID, securityGroupNode.ID, cluster.Spec.SecurityGroupNode.Rules)
	if err != nil {
		return cluster, fmt.Errorf("unable to build node security group rules: %w", err)
	}
	nodeRules, err := s.securityGroupSvc.UpsertSecurityGroupRules(ctx, securityGroupNode.ID, desiredNodeRulesList)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert node security group rules: %w", err)
	}
	cluster.Status.SecurityGroupNode.Rules = domainRulesToStatus(nodeRules)

	provisioned := true
	cluster.Status.Initialization.Provisioned = &provisioned

	return cluster, nil
}

func (s *clusterService) DeleteCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
	if cluster.Status.ControlPlaneEndpoint != nil && cluster.Status.ControlPlaneEndpoint.ID != "" {
		id, err := uuid.Parse(cluster.Status.ControlPlaneEndpoint.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse status.controlPlaneEndpoint.id: %w", err)
		}
		if err := s.elasticIPSvc.DeleteElasticIP(ctx, id); err != nil {
			return cluster, err
		}
	}

	// Purge rules before deleting SGs: cross-references between groups create circular dependencies
	// that prevent deletion until both are cleared.
	var securityGroupControlPlaneID *uuid.UUID
	if cluster.Status.SecurityGroupControlPlane != nil && cluster.Status.SecurityGroupControlPlane.ID != "" {
		id, err := uuid.Parse(cluster.Status.SecurityGroupControlPlane.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse status.securityGroupControlPlane.id: %w", err)
		}
		securityGroupControlPlaneID = &id
		if err := s.securityGroupSvc.PurgeSecurityGroup(ctx, id); err != nil {
			return cluster, err
		}
	}
	var securityGroupNodeID *uuid.UUID
	if cluster.Status.SecurityGroupNode != nil && cluster.Status.SecurityGroupNode.ID != "" {
		id, err := uuid.Parse(cluster.Status.SecurityGroupNode.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse status.securityGroupNode.id: %w", err)
		}
		securityGroupNodeID = &id
		if err := s.securityGroupSvc.PurgeSecurityGroup(ctx, id); err != nil {
			return cluster, err
		}
	}

	if securityGroupControlPlaneID != nil {
		if err := s.securityGroupSvc.DeleteSecurityGroup(ctx, *securityGroupControlPlaneID); err != nil {
			return cluster, fmt.Errorf("unable to delete security group for control plane: %w", err)
		}
	}
	if securityGroupNodeID != nil {
		if err := s.securityGroupSvc.DeleteSecurityGroup(ctx, *securityGroupNodeID); err != nil {
			return cluster, fmt.Errorf("unable to delete security group for node: %w", err)
		}
	}

	return cluster, nil
}
