package controller

type clusterDecisionInput struct {
	externallyManaged bool
	paused            bool
	deleting          bool
	hasFinalizer      bool
}

type clusterAction string

const (
	clusterActionSkipExternallyManaged clusterAction = "skip-externally-managed"
	clusterActionPause                 clusterAction = "pause"
	clusterActionReconcile             clusterAction = "reconcile"
	clusterActionDelete                clusterAction = "delete"
	clusterActionCompleteDeletion      clusterAction = "complete-deletion"
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
