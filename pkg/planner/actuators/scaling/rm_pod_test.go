package scaling

import (
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// rmPodActuatorFixture represents a fixture for testing.
type rmPodActuatorFixture struct {
	test    *testing.T
	client  *fake.Clientset
	objects []runtime.Object
}

// newRmPodActuatorFixture initializes a new fixture for testing.
func newRmPodActuatorFixture(t *testing.T) *rmPodActuatorFixture {
	f := &rmPodActuatorFixture{}
	f.test = t
	return f
}

// newRmPodTestActuator initializes an actuator for testing.
func (f *rmPodActuatorFixture) newRmPodTestActuator() *RmPodActuator {
	f.client = fake.NewSimpleClientset(f.objects...)
	cfg := RmPodConfig{
		LookBack: 20,
		MinPods:  1,
	}
	actuator := NewRmPodActuator(f.client, dummyTracer{}, cfg)
	return actuator
}

// Tests for success.

// TestRmNextStateForSuccess tests for success.
func TestRmNextStateForSuccess(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	start := common.State{
		Intent: common.Intent{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"default/p99": 20.0,
				"default/rps": 0.0,
			}},
		CurrentPods: map[string]common.PodState{
			"pod_0": {Availability: 0.8},
		},
	}
	goal := common.State{}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/rps": {ProfileType: common.ProfileTypeFromText("throughput"), Minimize: false},
	}
	actuator := f.newRmPodTestActuator()
	actuator.NextState(&start, &goal, profiles)
}

// TestRmPerformForSuccess tests for success.
func TestRmPerformForSuccess(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	f.objects = []runtime.Object{
		&coreV1.Pod{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "pod_0",
				Namespace: "default",
			},
		},
	}
	s0 := common.State{
		Intent: common.Intent{
			Key:        "my-deployment",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {Availability: 0.8},
		},
	}
	plan := []planner.Action{{Name: rmPodActionName, Properties: map[string]string{"name": "pod_0"}}}
	actuator := f.newRmPodTestActuator()
	actuator.Perform(&s0, plan)
}

// TestRmEffectForSuccess tests for success.
func TestRmEffectForSuccess(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	actuator := f.newRmPodTestActuator()
	actuator.Effect(&common.State{}, map[string]common.Profile{})
}

// Tests for failure.

// TestRmNextStateForFailure tests for failure.
func TestRmNextStateForFailure(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	actuator := f.newRmPodTestActuator()

	state := common.State{
		Intent: common.Intent{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"default/p99": 10.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {NodeName: "node0", Availability: 1.0, State: "Running"},
		},
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"default/p99": 3.0, "default/rps": 0.0, "default/availability": 0.999}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
	}

	// no throughput is being tracked.
	states, _, _ := actuator.NextState(&state, &goal, profiles)
	if len(states) > 0 {
		t.Errorf("Expected empty results set as we've not defined a throughput objective - got: %v", states)
	}

	// no data in knowledge base.
	profiles["default/throughput"] = common.Profile{ProfileType: common.ProfileTypeFromText("throughput")}
	profiles["default/blurb"] = common.Profile{ProfileType: common.ProfileTypeFromText("latency")}
	state.Intent.Objectives["default/blurb"] = 42.0
	state.Intent.Objectives["default/throughput"] = 200.0
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(states) > 0 {
		t.Errorf("Expected empty results set as knowledge base is corrupt/empty. - got: %v", states)
	}
}

// TestRmPerformForFailure tests for failure.
func TestRmPerformForFailure(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	f.objects = []runtime.Object{}
	s0 := common.State{
		Intent: common.Intent{
			Key:        "my-deployment",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {Availability: 0.8},
		},
	}
	plan := []planner.Action{{Name: rmPodActionName, Properties: map[string]string{"name": "pod_0"}}}
	actuator := f.newRmPodTestActuator()
	actuator.Perform(&s0, plan)
	if len(f.client.Actions()) != 1 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}
}

// Tests for sanity.

func TestRmNextStateForSanity(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	start := common.State{
		Intent: common.Intent{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"default/p99":          20.0,
				"default/rps":          100.0,
				"default/availability": 0.996,
			}},
		CurrentPods: map[string]common.PodState{
			"pod_0":     {Availability: 0.8, State: "Running"},
			"pod_1":     {Availability: 0.8, State: "Terminating"},
			"dummy@abc": {Availability: 0.96},
		},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	profiles := map[string]common.Profile{
		"default/p99":          {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/rps":          {ProfileType: common.ProfileTypeFromText("throughput"), Minimize: false},
		"default/availability": {ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
	}
	actuator := f.newRmPodTestActuator()
	states, utilities, actions := actuator.NextState(&start, &goal, profiles)
	if len(states) != len(utilities) || len(utilities) != len(actions) {
		t.Errorf("All resultsets should equal in length: %v, %v, %v", states, utilities, actions)
	}
	if len(states) != 1 {
		t.Errorf("Expected only pod0 to be removed...: %v.", states)
	}
	if utilities[0] != 0.992 {
		t.Errorf("Expected the following utilities [0.992] - got: %v.", utilities)
	}
}

// TestRmPerformForSuccess tests for sanity.
func TestRmPerformForSanity(t *testing.T) {
	f := newRmPodActuatorFixture(t)
	f.objects = []runtime.Object{
		&coreV1.Pod{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "pod_0",
				Namespace: "default",
			},
		},
	}
	s0 := common.State{
		Intent: common.Intent{
			Key:        "my-deployment",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {Availability: 0.8},
			"pod_1": {Availability: 0.8},
		},
	}
	plan := []planner.Action{{Name: rmPodActionName, Properties: map[string]string{"name": "pod_0"}}}
	actuator := f.newRmPodTestActuator()
	actuator.Perform(&s0, plan)
	expectedActions := []string{"delete"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
}
