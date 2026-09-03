package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/infrastructure/exoscale"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	zone := flag.String("zone", "", "Exoscale zone")
	instance := flag.String("instance", "", "instance ID expected to be absent")
	elasticIP := flag.String("elastic-ip", "", "Elastic IP ID expected to be absent")
	securityGroups := flag.String("security-groups", "", "comma-separated security group IDs expected to be absent")
	flag.Parse()

	cloud, err := exoscale.NewCloud(
		os.Getenv("EXOSCALE_API_KEY"), os.Getenv("EXOSCALE_API_SECRET"), egoscale.ZoneName(*zone),
	)
	if err != nil {
		return err
	}
	ctx := context.Background()

	if err := checkAbsent(*instance, domain.ErrInstanceNotFound, func(id uuid.UUID) error {
		_, err := cloud.GetInstance(ctx, id)
		return err
	}); err != nil {
		return fmt.Errorf("instance: %w", err)
	}
	if err := checkAbsent(*elasticIP, domain.ErrElasticIPNotFound, func(id uuid.UUID) error {
		_, err := cloud.GetElasticIP(ctx, id)
		return err
	}); err != nil {
		return fmt.Errorf("elastic IP: %w", err)
	}
	for rawID := range strings.SplitSeq(*securityGroups, ",") {
		if err := checkAbsent(rawID, domain.ErrSecurityGroupNotFound, func(id uuid.UUID) error {
			_, err := cloud.GetSecurityGroup(ctx, id)
			return err
		}); err != nil {
			return fmt.Errorf("security group: %w", err)
		}
	}
	return nil
}

func checkAbsent(rawID string, notFound error, get func(uuid.UUID) error) error {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return err
	}
	if err := get(id); !errors.Is(err, notFound) {
		if err == nil {
			return fmt.Errorf("%s still exists", id)
		}
		return err
	}
	return nil
}
