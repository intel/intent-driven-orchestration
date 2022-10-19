package planner

import (
	"github.com/intel/intent-driven-orchestration/pkg/common"
)

// Action holds information for a particular action.
type Action struct {
	Name       string
	Properties interface{}
}

// Planner represents the basic interface all planners should adhere too.
type Planner interface {
	// CreatePlan creates a plan based on the given current and desired state.
	CreatePlan(current common.State, desired common.State, profiles map[string]common.Profile) []Action
	// ExecutePlan triggers the planner to actually perform the Plan.
	ExecutePlan(common.State, []Action)
	// TriggerEffect triggers all actuators planning actuators to (optionally) reflect on the effect of their actions.
	TriggerEffect(current common.State, profiles map[string]common.Profile)
}
