package main

import (
	"errors"
	"testing"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCheckAbsent(t *testing.T) {
	id := uuid.NewString()

	assert.NoError(t, checkAbsent(id, domain.ErrInstanceNotFound, func(uuid.UUID) error {
		return domain.ErrInstanceNotFound
	}))
	assert.ErrorContains(t, checkAbsent(id, domain.ErrInstanceNotFound, func(uuid.UUID) error {
		return nil
	}), "still exists")
	assert.Error(t, checkAbsent("invalid", domain.ErrInstanceNotFound, func(uuid.UUID) error {
		return errors.New("unreachable")
	}))
}
