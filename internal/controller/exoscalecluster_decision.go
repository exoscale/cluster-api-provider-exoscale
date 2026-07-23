package controller

type clusterDecisionInput struct {
	externallyManaged bool
	paused            bool
	deleting          bool
	hasFinalizer      bool
}

type clusterAction uint8

const (
	clusterActionSkipExternallyManaged clusterAction = iota
	clusterActionPause
	clusterActionReconcile
	clusterActionDelete
	clusterActionCompleteDeletion
)

func decideCluster(input clusterDecisionInput) clusterAction {
	switch {
	case input.externallyManaged:
		return clusterActionSkipExternallyManaged
	case input.paused:
		return clusterActionPause
	case !input.deleting:
		return clusterActionReconcile
	case input.hasFinalizer:
		return clusterActionDelete
	default:
		return clusterActionCompleteDeletion
	}
}
