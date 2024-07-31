package common

import (
	"math"
	"strings"
	"time"
)

// ProfileType defines the type of KPIProfiles.
type ProfileType int

const (
	Obsolete ProfileType = iota
	Latency
	Availability
	Throughput
	Power
)

// ProfileTypeFromText converts string into the right int.
func ProfileTypeFromText(text string) ProfileType {
	switch strings.ToLower(text) {
	default:
		return Obsolete
	case "latency":
		return Latency
	case "availability":
		return Availability
	case "throughput":
		return Throughput
	case "power":
		return Power
	}
}

// PodError holds start and end time for an error of a POD.
type PodError struct {
	Key     string
	Start   time.Time
	End     time.Time
	Created time.Time
	// TODO: look into adding reason for failure etc.
}

// Profile holds information about valid objective profiles.
type Profile struct {
	Key         string
	ProfileType ProfileType
	Query       string
	External    bool
	Address     string
}

// Intent holds information about an intent in the system.
type Intent struct {
	Key        string
	Priority   float64
	TargetKey  string
	TargetKind string
	Objectives map[string]float64
}

// PodState represents the state of an POD.
type PodState struct {
	Availability float64
	NodeName     string
	State        string
	QoSClass     string
}

// State represents the state a set of PODs can be in.
type State struct {
	Intent      Intent
	CurrentPods map[string]PodState
	CurrentData map[string]map[string]float64
	Resources   map[string]int64
	Annotations map[string]string
}

// DeepCopy creates a deep copy of a state.
func (one *State) DeepCopy() State {
	// make sure we capture the objectives.
	objective := Intent{
		Key:        one.Intent.Key,
		Priority:   one.Intent.Priority,
		TargetKey:  one.Intent.TargetKey,
		TargetKind: one.Intent.TargetKind,
		Objectives: map[string]float64{},
	}
	for k, v := range one.Intent.Objectives {
		objective.Objectives[k] = v
	}

	// new state.
	tmp := State{
		objective,
		map[string]PodState{},
		map[string]map[string]float64{},
		map[string]int64{},
		map[string]string{},
	}

	// copy over pod states.
	pods := map[string]PodState{}
	for k, v := range one.CurrentPods {
		state := PodState{
			v.Availability,
			v.NodeName,
			v.State,
			v.QoSClass,
		}
		pods[k] = state
	}
	tmp.CurrentPods = pods

	// annotations and resources
	if one.Resources == nil {
		tmp.Resources = nil
	} else {
		for rk, rv := range one.Resources {
			tmp.Resources[rk] = rv
		}
	}
	if one.Annotations == nil {
		tmp.Annotations = nil
	} else {
		for ak, av := range one.Annotations {
			tmp.Annotations[ak] = av
		}
	}

	// and all the rest.
	for k := range one.CurrentData {
		subMap := map[string]float64{}
		for a, b := range one.CurrentData[k] {
			subMap[a] = b
		}
		tmp.CurrentData[k] = subMap
	}
	return tmp
}

// Distance calculates the euclidean Distance between two states. Potentially we can add weights here maybe - or another way to calculate the Distance so e.g. the Distance for P99 is more important than the Distance in P50 latency.
func (one *State) Distance(another *State, profiles map[string]Profile) float64 {
	squaresSum := 0.0
	for key, val := range one.Intent.Objectives {
		squaresSum += math.Pow(val-another.Intent.Objectives[key], 2)
	}
	if one.IsBetter(another, profiles) && squaresSum != 0.0 {
		// we should favor states which are closer to the goal...
		return -1 / math.Sqrt(squaresSum)
	}
	return math.Sqrt(squaresSum)
}

// IsBetter compares the objectives of one state to another - returns true if all latency or power related objective targets are smaller or equal, and all others are larger or equal.
func (one *State) IsBetter(another *State, profiles map[string]Profile) bool {
	if len(one.Intent.Objectives) != len(another.Intent.Objectives) {
		return false
	}
	res := false
	for k, v := range one.Intent.Objectives {
		// TODO: make this configurable through the KPI profiles for which we define larger or smaller is better.
		if profiles[k].ProfileType == ProfileTypeFromText("latency") || profiles[k].ProfileType == ProfileTypeFromText("power") {
			if v <= another.Intent.Objectives[k] {
				res = true
			} else {
				return false
			}
		} else {
			if v >= another.Intent.Objectives[k] {
				res = true
			} else {
				return false
			}
		}
	}
	return res
}

// LessResources contrast the resources and returns true if one state has less resource than another.
func (one *State) LessResources(another *State) bool {
	res := false
	for k, v := range one.Resources {
		tmp, ok := another.Resources[k]
		if !ok {
			return false
		}
		if v <= tmp {
			res = true
		} else {
			return false
		}
	}

	return res
}
