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
	cloudClient, err := exoscale.NewCloud(apiKey, apisecret, zone)
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

	var securityGroupWorkerID *uuid.UUID
	var securityGroupWorkerName = fmt.Sprintf("capi - %s - worker", clusterID.String())

	if cluster.Status.SecurityGroupWorker != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupWorker.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.SecurityGroupWorker.id", errInvalidID, err)
		}
		securityGroupWorkerID = &id
	}
	securityGroupWorker, err := s.securityGroupSvc.UpsertSecurityGroup(ctx, clusterID, securityGroupWorkerID, securityGroupWorkerName)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert security group worker: %w", err)
	}
	cluster.Status.SecurityGroupWorker = &infrav1alpha1.SecurityGroupStatus{
		ID:   securityGroupWorker.ID.String(),
		Name: securityGroupWorker.Name,
	}

	/*
	** Security Group Rules
	 */
	desiredCPRules, err := resolveRules(securityGroupControlPlane.ID, securityGroupWorker.ID, defaultControlPlaneRules(cluster.Spec.ControlPlaneEndpoint.Port), cluster.Spec.SecurityGroupControlPlane.Rules)
	if err != nil {
		return cluster, fmt.Errorf("unable to build control plane security group rules: %w", err)
	}
	cpRules, err := s.securityGroupSvc.UpsertSecurityGroupRules(ctx, securityGroupControlPlane.ID, desiredCPRules)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert control plane security group rules: %w", err)
	}
	cluster.Status.SecurityGroupControlPlan.Rules = domainRulesToStatus(cpRules)

	desiredWorkerRulesList, err := resolveRules(securityGroupControlPlane.ID, securityGroupWorker.ID, defaultWorkerRules(), cluster.Spec.SecurityGroupWorker.Rules)
	if err != nil {
		return cluster, fmt.Errorf("unable to build worker security group rules: %w", err)
	}
	WorkerRules, err := s.securityGroupSvc.UpsertSecurityGroupRules(ctx, securityGroupWorker.ID, desiredWorkerRulesList)
	if err != nil {
		return cluster, fmt.Errorf("unable to upsert worker security group rules: %w", err)
	}
	cluster.Status.SecurityGroupWorker.Rules = domainRulesToStatus(WorkerRules)

	cluster.Status.Initialization.Provisioned = new(true)

	return cluster, nil
}

func (s *clusterService) DeleteCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
	if cluster.Status.ControlPlaneEndpoint != nil {
		id, err := uuid.Parse(cluster.Status.ControlPlaneEndpoint.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.controlPlaneEndpoint.id", errInvalidID, err)
		}
		if err := s.elasticIPSvc.DeleteElasticIP(ctx, id); err != nil {
			return cluster, err
		}
	}

	// Rules must be purged from all security groups before deleting them. Security groups can
	// reference each other in their rules (e.g. group A has a rule pointing to group B, and group B
	// has a rule pointing to group A), creating a circular dependency that prevents deletion of either
	// group until both are fully cleared of their rules.
	var securityGroupControlPlaneID *uuid.UUID
	if cluster.Status.SecurityGroupControlPlan != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupControlPlan.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.securityGroupControlPlane.id", errInvalidID, err)
		}
		securityGroupControlPlaneID = &id
		if err := s.securityGroupSvc.PurgeSecurityGroup(ctx, id); err != nil {
			return cluster, err
		}
	}
	var securityGroupWorkerID *uuid.UUID
	if cluster.Status.SecurityGroupWorker != nil {
		id, err := uuid.Parse(cluster.Status.SecurityGroupWorker.ID)
		if err != nil {
			return cluster, fmt.Errorf("unable to parse %q: %w: %w", ".status.securityGroupWorker.id", errInvalidID, err)
		}
		securityGroupWorkerID = &id
		if err := s.securityGroupSvc.PurgeSecurityGroup(ctx, id); err != nil {
			return cluster, err
		}
	}

	if securityGroupControlPlaneID != nil {
		if err := s.securityGroupSvc.DeleteSecurityGroup(ctx, *securityGroupControlPlaneID); err != nil {
			return cluster, fmt.Errorf("unable to delete security group for control plane: %w", err)
		}
	}
	if securityGroupWorkerID != nil {
		if err := s.securityGroupSvc.DeleteSecurityGroup(ctx, *securityGroupWorkerID); err != nil {
			return cluster, fmt.Errorf("unable to delete security group for worker: %w", err)
		}
	}

	return cluster, nil
}
