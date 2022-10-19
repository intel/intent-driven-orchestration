package actuators

import (
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
)

// Plugin defines that actuators have names and are grouped.
type Plugin interface {
	// Name of the actuator.
	Name() string
	// Group this actuator is part of.
	Group() string
}

// ActuatorPlugin defines the interface for the actuators.
type Actuator interface {
	Plugin
	// NextState should return a set of potential follow-up states for a given state if this actuator would potentially be used.
	NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action)
	// Perform should perform those actions of the plan that it is in charge off.
	Perform(state *common.State, plan []planner.Action)
	// Effect should (optionally) recalculate the effect this actuator has for ALL objectives for this workload.
	Effect(state *common.State, profiles map[string]common.Profile)
}
