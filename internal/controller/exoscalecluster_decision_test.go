package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecideCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input clusterDecisionInput
		want  clusterAction
	}{
		{
			name:  "externally managed takes precedence",
			input: clusterDecisionInput{externallyManaged: true, paused: true, deleting: true, hasFinalizer: true},
			want:  clusterActionSkipExternallyManaged,
		},
		{
			name:  "paused takes precedence over deletion",
			input: clusterDecisionInput{paused: true, deleting: true, hasFinalizer: true},
			want:  clusterActionPause,
		},
		{
			name:  "active cluster reconciles",
			input: clusterDecisionInput{hasFinalizer: true},
			want:  clusterActionReconcile,
		},
		{
			name:  "deleting cluster with finalizer deletes cloud resources",
			input: clusterDecisionInput{deleting: true, hasFinalizer: true},
			want:  clusterActionDelete,
		},
		{
			name:  "deleting cluster without finalizer completes",
			input: clusterDecisionInput{deleting: true},
			want:  clusterActionCompleteDeletion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, decideCluster(tt.input))
		})
	}
}
