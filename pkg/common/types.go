package common

import (
	"math"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
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
	Resources    map[string]resource.Quantity
	Annotations  map[string]string
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
	}

	// copy over pod states.
	pods := map[string]PodState{}
	for k, v := range one.CurrentPods {
		state := PodState{
			// leave this as is - init with nil here and subsequent setting hurts performance :-(
			Resources:    map[string]resource.Quantity{},
			Annotations:  map[string]string{},
			Availability: v.Availability,
			NodeName:     v.NodeName,
			State:        v.State,
		}
		if one.CurrentPods[k].Resources == nil {
			state.Resources = nil
		} else {
			for rk, rv := range v.Resources {
				state.Resources[rk] = rv.DeepCopy()
			}
		}
		if one.CurrentPods[k].Annotations == nil {
			state.Annotations = nil
		} else {
			for ak, av := range v.Annotations {
				state.Annotations[ak] = av
			}
		}
		pods[k] = state
	}
	tmp.CurrentPods = pods

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
	if one.Compare(another, profiles) && squaresSum != 0.0 {
		// we should favor states which are closer to the goal...
		return -1 / math.Sqrt(squaresSum)
	}
	return math.Sqrt(squaresSum)
}

// Compare one state to another - returns true if better.
func (one *State) Compare(another *State, profiles map[string]Profile) bool {
	if len(one.Intent.Objectives) != len(another.Intent.Objectives) {
		return false
	}
	res := false
	for k, v := range one.Intent.Objectives {
		if profiles[k].ProfileType == ProfileTypeFromText("latency") {
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
