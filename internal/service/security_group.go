package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

type securityGroupService struct {
	cloud  domain.Cloud
	logger logr.Logger
}

func NewSecurityGroupService(client domain.Cloud, logger logr.Logger) *securityGroupService {
	return &securityGroupService{cloud: client, logger: logger}
}

func (s securityGroupService) UpsertSecurityGroup(ctx context.Context, clusterID uuid.UUID, scID *uuid.UUID, name string) (domain.SecurityGroup, error) {
	if scID != nil {
		sc, err := s.cloud.GetSecurityGroup(ctx, *scID)
		if err != nil && !errors.Is(err, domain.ErrSecurityGroupNotFound) {
			return domain.SecurityGroup{}, fmt.Errorf("error while fetching security group: %w", err)
		} else if errors.Is(err, domain.ErrSecurityGroupNotFound) {
			s.logger.Info("Security group not found, will create a new one")

			id, err := s.cloud.CreateSecurityGroup(ctx, name)
			if err != nil {
				return domain.SecurityGroup{}, fmt.Errorf("error while creating security group: %w", err)
			}
			sc, err = s.cloud.GetSecurityGroup(ctx, id)
			if err != nil {
				return domain.SecurityGroup{}, fmt.Errorf("error while fetching new security group, %w", err)
			}

			return sc, nil
		}

		return sc, nil
	} else {
		existing, err := s.FindSecurityGroup(ctx, name)
		if err != nil && !errors.Is(err, domain.ErrSecurityGroupNotFound) {
			return domain.SecurityGroup{}, fmt.Errorf("error while searching for existing security group: %w", err)
		} else if err == nil {
			s.logger.Info("Found an existing security group by name, reusing it", "securityGroupID", existing.ID.String())
			return existing, nil
		}

		s.logger.Info("Create security group")

		id, err := s.cloud.CreateSecurityGroup(ctx, name)
		if err != nil {
			return domain.SecurityGroup{}, fmt.Errorf("error while creating security group: %w", err)
		}
		sc, err := s.cloud.GetSecurityGroup(ctx, id)
		if err != nil {
			return domain.SecurityGroup{}, fmt.Errorf("error while fetching new security group, %w", err)
		}

		return sc, nil
	}
}

func (s securityGroupService) FindSecurityGroup(ctx context.Context, name string) (domain.SecurityGroup, error) {
	securityGroups, err := s.cloud.ListSecurityGroups(ctx)
	if err != nil {
		return domain.SecurityGroup{}, err
	}
	var match *domain.SecurityGroup
	for i := range securityGroups {
		if securityGroups[i].Name != name {
			continue
		}
		if match != nil {
			return domain.SecurityGroup{}, fmt.Errorf("multiple security groups named %q", name)
		}
		match = &securityGroups[i]
	}
	if match == nil {
		return domain.SecurityGroup{}, domain.ErrSecurityGroupNotFound
	}
	return *match, nil
}

func (s *securityGroupService) DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error {
	if _, err := s.cloud.GetSecurityGroup(ctx, id); err != nil {
		if errors.Is(err, domain.ErrSecurityGroupNotFound) {
			return nil
		}
		return err
	}

	s.logger.Info("Delete security group", "securityGroupID", id.String())
	if err := s.cloud.DeleteSecurityGroup(ctx, id); err != nil {
		return err
	}

	return nil
}

func (s *securityGroupService) UpsertSecurityGroupRules(ctx context.Context, sgID uuid.UUID, desiredRules []domain.SecurityGroupRule) ([]domain.SecurityGroupRule, error) {
	existingRules, err := s.cloud.ListSecurityGroupRules(ctx, sgID)
	if err != nil {
		return nil, fmt.Errorf("error listing security group rules: %w", err)
	}

	toAdd, toDelete := diffRules(desiredRules, existingRules)

	for _, rule := range toDelete {
		s.logger.Info("Deleting security group rule", "securityGroupID", sgID.String(), "ruleID", rule.ID)
		if err := s.cloud.DeleteSecurityGroupRule(ctx, sgID, rule.ID); err != nil {
			return nil, fmt.Errorf("error deleting security group rule %q: %w", fmt.Sprintf("%s/%s", sgID.String(), rule.ID.String()), err)
		}
	}

	for i := range toAdd {
		s.logger.Info("Creating security group rule", "description", toAdd[i].Description)
		id, err := s.cloud.CreateSecurityGroupRule(ctx, sgID, toAdd[i])
		if err != nil {
			return nil, fmt.Errorf("error creating security group rule: %w", err)
		}
		toAdd[i].ID = id
	}

	deletedIDs := make(map[uuid.UUID]struct{}, len(toDelete))
	for _, r := range toDelete {
		deletedIDs[r.ID] = struct{}{}
	}

	finalRules := make([]domain.SecurityGroupRule, 0, len(existingRules)-len(toDelete)+len(toAdd))
	for _, r := range existingRules {
		if _, deleted := deletedIDs[r.ID]; !deleted {
			finalRules = append(finalRules, r)
		}
	}
	finalRules = append(finalRules, toAdd...)

	return finalRules, nil
}

func (s *securityGroupService) PurgeSecurityGroup(ctx context.Context, sgID uuid.UUID) error {
	s.logger.Info("Purge security group from rule", "securityGroupID", sgID.String())
	rules, err := s.cloud.ListSecurityGroupRules(ctx, sgID)
	if err != nil {
		if errors.Is(err, domain.ErrSecurityGroupNotFound) {
			return nil
		}
		return fmt.Errorf("unable to list rules from security group: %q: %w", sgID.String(), err)
	}

	for _, rule := range rules {
		if err := s.cloud.DeleteSecurityGroupRule(ctx, sgID, rule.ID); err != nil {
			return fmt.Errorf("unable to delete security group rule: %q: %w", fmt.Sprintf("%s/%s", sgID.String(), rule.ID.String()), err)
		}
	}

	return nil
}
