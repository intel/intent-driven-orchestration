package plugins

import (
	protobufs "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1/protobufs"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
)

type ActuatorValidationSet struct {
	start     *common.State
	goal      *common.State
	profiles  map[string]common.Profile
	end       []common.State
	utilities []float64
	actions   []planner.Action
}

type ActuatorGrpcValidationSet struct {
	start     *protobufs.State
	goal      *protobufs.State
	profiles  map[string]*protobufs.Profile
	end       []*protobufs.State
	utilities []float64
	actions   []*protobufs.Action
}

func generateActuatorGrpcValidationSet() *ActuatorGrpcValidationSet {
	vSet := ActuatorGrpcValidationSet{
		start: &protobufs.State{
			Intent: &protobufs.Intent{
				Key:        "test-my-objective",
				Priority:   0.0,
				TargetKey:  "my-deployment",
				TargetKind: "Deployment",
				Objectives: map[string]float64{
					"p99latency": 150,
				},
			},
			CurrentPods: map[string]*protobufs.PodState{"pod_0": {Availability: 0.7}},
			CurrentData: map[string]*protobufs.DataEntry{"cpu_value": {
				Data: map[string]float64{"host0": 20.0},
			},
			},
			Resources:   map[string]int64{"cpu": 23},
			Annotations: map[string]string{"foo": "bar"},
		},
		goal: &protobufs.State{
			Intent: &protobufs.Intent{
				Key:        "goal",
				Priority:   0.23,
				TargetKey:  "my-deployment",
				TargetKind: "Deployment",
				Objectives: map[string]float64{
					"p99latency": 100,
				}},
			CurrentPods: nil,
			CurrentData: nil,
		},
		profiles: map[string]*protobufs.Profile{"p99latency": {ProfileType: protobufs.ProfileType_LATENCY}},
		end: []*protobufs.State{
			{
				Intent: &protobufs.Intent{
					Key:        "end-objective",
					Priority:   0.2,
					TargetKey:  "my-deployment",
					TargetKind: "Deployment",
					Objectives: map[string]float64{
						"p99latency": 222,
					},
				},
				CurrentPods: map[string]*protobufs.PodState{"pod_0": {Availability: 0.6}},
				CurrentData: map[string]*protobufs.DataEntry{"cpu_value": {Data: map[string]float64{"host0": 21.3}}},
				Resources:   map[string]int64{"cpu": 12},
				Annotations: map[string]string{"foo": "bar"},
			},
		},
		utilities: []float64{0.32, 0.64},
		actions: []*protobufs.Action{
			{
				Name: "action 1",
				Properties: &protobufs.ActionProperties{
					Type:          protobufs.PropertyType_STRING_PROPERTY,
					StrProperties: map[string]string{"option1": "v_a", "option2": "v_b"},
				},
			},
			{
				Name: "action 2",
				Properties: &protobufs.ActionProperties{
					Type:          protobufs.PropertyType_INT_PROPERTY,
					IntProperties: map[string]int64{"option3": 42},
				},
			},
		},
	}
	return &vSet
}

func generateActuatorValidationSet() *ActuatorValidationSet {
	vSet := ActuatorValidationSet{
		start: &common.State{
			Intent: common.Intent{
				Key:        "test-my-objective",
				Priority:   0.0,
				TargetKey:  "my-deployment",
				TargetKind: "Deployment",
				Objectives: map[string]float64{
					"p99latency": 150,
				},
			},
			CurrentPods: map[string]common.PodState{"pod_0": {Availability: 0.7}},
			CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20.0}},
			Resources:   map[string]int64{"cpu": 23},
			Annotations: map[string]string{"foo": "bar"},
		},
		goal: &common.State{
			Intent: common.Intent{
				Key:        "goal",
				Priority:   0.23,
				TargetKey:  "my-deployment",
				TargetKind: "Deployment",
				Objectives: map[string]float64{
					"p99latency": 100,
				}},
			CurrentPods: nil,
			CurrentData: nil,
		},
		profiles: map[string]common.Profile{"p99latency": {ProfileType: common.ProfileTypeFromText("latency")}},
		end: []common.State{
			{
				Intent: common.Intent{
					Key:        "end-objective",
					Priority:   0.2,
					TargetKey:  "my-deployment",
					TargetKind: "Deployment",
					Objectives: map[string]float64{
						"p99latency": 222,
					},
				},
				CurrentPods: map[string]common.PodState{"pod_0": {Availability: 0.6}},
				CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 21.3}},
				Resources:   map[string]int64{"cpu": 12},
				Annotations: map[string]string{"foo": "bar"},
			},
		},
		utilities: []float64{0.32, 0.64},
		actions: []planner.Action{
			{
				Name:       "action 1",
				Properties: map[string]string{"option1": "v_a", "option2": "v_b"},
			},
			{
				Name:       "action 2",
				Properties: map[string]int64{"option3": 42},
			},
		},
	}
	return &vSet
}
