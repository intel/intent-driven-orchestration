package platform

import (
	"os/exec"
	"testing"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

// dummyTracer allows us to control what information to give to the actuator.
type dummyTracer struct{}

func (d dummyTracer) TraceEvent(_ common.State, _ common.State, _ []planner.Action) {
	klog.Fatalf("Not needed.")
}

func (d dummyTracer) GetEffect(_ string, _ string, _ string, _ int, _ func() interface{}) (interface{}, error) {
	return nil, nil
}

// rdtActuatorFixture represents a fixture for testing.
type rdtActuatorFixture struct {
	client  *fake.Clientset
	objects []runtime.Object
}

// newRdtActuatorFixture initializes a new text fixture.
func newRdtActuatorFixture() *rdtActuatorFixture {
	f := &rdtActuatorFixture{}
	return f
}

// newRdtTestActuator initializes an actuator for testing.
func (f *rdtActuatorFixture) newRdtTestActuator() *RdtActuator {
	f.client = fake.NewSimpleClientset(f.objects...)
	cfg := RdtConfig{
		Interpreter: "python3",
		Analytics:   "test_analyze.py",
		Prediction:  "test_predict.py",
		Options:     []string{"None", "option_a", "option_b", "option_c"},
	}
	actuator := NewRdtActuator(f.client, dummyTracer{}, cfg)

	cmd := exec.Command(actuator.config.Interpreter, actuator.config.Prediction) //#nosec G204 -- NA
	err := cmd.Start()
	if err != nil {
		klog.Errorf("Could not start the prediction script: %s.", err)
	}
	// looks like we need this on slower boxes...
	time.Sleep(500 * time.Millisecond)

	return actuator
}

// Tests for success.

// TestRdtNextStateForSuccess tests for success.
func TestRdtNextStateForSuccess(t *testing.T) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()

	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			Objectives map[string]float64
		}{
			Key:        "default/function-intents",
			Priority:   1.0,
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"default/p99": 20.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {},
		},
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"default/p99": 10.0}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency")},
	}

	actuator.NextState(&state, &goal, profiles)
}

// TestRdtPerformForSuccess tests for success.
func TestRdtPerformForSuccess(t *testing.T) {
	f := newRdtActuatorFixture()
	f.objects = []runtime.Object{
		&coreV1.Pod{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "pod_0",
				Namespace: "default",
			},
		},
	}
	actuator := f.newRdtTestActuator()
	state := common.State{
		Intent:      common.Intent{TargetKey: "default/my-function", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	var plan []planner.Action
	actuator.Perform(&state, plan)
}

// TestRdtEffectForSuccess tests for success.
func TestRdtEffectForSuccess(t *testing.T) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()
	state := common.State{
		Intent: common.Intent{
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"default/p99": 10.0,
			},
		},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	profiles := map[string]common.Profile{
		"default/p99":        {ProfileType: common.ProfileTypeFromText("latency")},
		"default/throughput": {ProfileType: common.ProfileTypeFromText("throughput")},
	}
	actuator.Effect(&state, profiles)
}

// Tests for failure.

// TestRdtPerformForFailure tests for failure.
func TestRdtPerformForFailure(t *testing.T) {
	f := newRdtActuatorFixture()
	f.objects = []runtime.Object{}
	actuator := f.newRdtTestActuator()
	state := common.State{
		Intent: common.Intent{
			TargetKey: "default/my-function",
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {},
		},
	}
	plan := []planner.Action{
		{Name: rdtActionName, Properties: map[string]string{"option": "option_a"}},
	}

	// should fail.
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 1 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}
}

// TestRdtEffectForFailure tests for failure.
func TestRdtEffectForFailure(t *testing.T) {
	f := newRdtActuatorFixture()
	// not much to do here, as this will "just" trigger a python script.
	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			Objectives map[string]float64
		}{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"p99": 20.0,
			},
		},
	}
	profiles := map[string]common.Profile{
		"p99": {ProfileType: common.ProfileTypeFromText("latency")},
	}
	actuator := f.newRdtTestActuator()
	actuator.config.Analytics = "foobar"
	// will cause a logging warning.
	actuator.Effect(&state, profiles)
}

// Tests for sanity.

// TestRdtNextStateForSanity tests for sanity.
func TestRdtNextStateForSanity(t *testing.T) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()

	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			Objectives map[string]float64
		}{
			Key:        "default/function-intents",
			Priority:   1.0,
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"default/p99":   10.0,
				"default/blurb": 10.0,
			},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {Annotations: map[string]string{"foo": "bar"}},
		},
		CurrentData: map[string]map[string]float64{
			"cpu_value": {"node": 10.0},
		},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	goal.Intent.Objectives = map[string]float64{
		"default/p99":   5.0,
		"default/blurb": 10.0,
	}
	profiles := map[string]common.Profile{
		"default/p99":   {ProfileType: common.ProfileTypeFromText("latency")},
		"default/blurb": {ProfileType: common.ProfileTypeFromText("availability")},
	}

	states, utils, actions := actuator.NextState(&state, &goal, profiles)
	if len(states) != 1 || len(utils) != 1 || len(actions) != 1 {
		t.Errorf("Expected one entry each: %v, %v, %v.", states, utils, actions)
	}
	// check if annotations are set.
	for _, pod := range states[0].CurrentPods {
		if _, found := pod.Annotations["rdt_visited"]; !found {
			t.Errorf("Expected the temp blocker in annotation:  %v.", pod.Annotations)
		}
	}
	// check if resulting action match.
	if actions[0].Name != actuator.Name() || actions[0].Properties.(map[string]string)["option"] != "option_b" {
		t.Errorf("Expected a action to set option_b - got: %v.", actions[0])
	}
	// check utility...
	if utils[0] != 1.0 {
		t.Errorf("Expected util to be 1.0 - got: %v.", utils)
	}

	// expect empty if no solution can be found.
	goal.Intent.Objectives["default/p99"] = 1.0
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(states) > 0 {
		t.Errorf("Expected no results - got: %v.", states)
	}

	// expect empty res if we are good for now.
	state.Intent.Objectives["default/p99"] = 4.9
	goal.Intent.Objectives["default/p99"] = 5.0
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) > 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// expect change of clos if we change latency target
	state.Intent.Objectives["default/p99"] = 4.9
	goal.Intent.Objectives["default/p99"] = 15.0
	_, utils, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 1 || actions[0].Properties.(map[string]string)["option"] != "option_a" {
		t.Errorf("Expected option_a - got: %v.", actions)
	}
	// check utility...
	if utils[0] >= 0.7 {
		t.Errorf("Expected util < 0.7 - got: %v.", utils)
	}

	// we do not want to revisit this action if we've already looked at it.
	state.CurrentPods["pod_0"].Annotations["rdt_visited"] = ""
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) > 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// we do not expect to set an option if it is already set...
	state.CurrentPods["pod_0"].Annotations["rdt_config"] = "option_a"
	delete(state.CurrentPods["pod_0"].Annotations, "rdt_visited")
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// expect empty is script return -1.0
	state.Intent.Objectives["default/p72"] = 10.0
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// TODO: test proactive action setting.
}

// BenchmarkNextState benchmarks.
func BenchmarkNextState(b *testing.B) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()

	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			Objectives map[string]float64
		}{
			Key:        "default/function-intents",
			Priority:   1.0,
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"default/p99":   10.0,
				"default/blurb": 10.0,
			},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {Annotations: map[string]string{"foo": "bar"}},
		},
		CurrentData: map[string]map[string]float64{
			"cpu_value": {"node": 10.0},
		},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	goal.Intent.Objectives = map[string]float64{
		"default/p99":   5.0,
		"default/blurb": 10.0,
	}
	profiles := map[string]common.Profile{
		"default/p99":   {ProfileType: common.ProfileTypeFromText("latency")},
		"default/blurb": {ProfileType: common.ProfileTypeFromText("availability")},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		states, _, _ := actuator.NextState(&state, &goal, profiles)
		if len(states) != 1 {
			b.Errorf("Expected 1 - got: %d.", len(states))
		}
	}
}

// TestRdtPerformForSanity tests for sanity.
func TestRdtPerformForSanity(t *testing.T) {
	f := newRdtActuatorFixture()
	f.objects = []runtime.Object{
		&coreV1.Pod{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "pod_0",
				Namespace: "default",
			},
		},
	}
	actuator := f.newRdtTestActuator()
	state := common.State{
		Intent: common.Intent{
			TargetKey: "default/my-function",
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {},
		},
	}
	plan := []planner.Action{
		{Name: rdtActionName, Properties: map[string]string{"option": "option_a"}},
	}

	// check if annotation clos_option it set on pod.
	actuator.Perform(&state, plan)
	expectedActions := []string{"get", "update"}
	j := 0
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
		j++
	}
	if j != len(expectedActions) {
		t.Errorf("Expected more actions: %v - found only: %d.", expectedActions, j)
	}

	// check if annotation is removed if option is None:
	f.client.ClearActions()
	plan = []planner.Action{
		{Name: rdtActionName, Properties: map[string]string{"option": "None"}},
	}

	// check if annotation clos_option it set on pod.
	actuator.Perform(&state, plan)
	expectedActions = []string{"get", "update"}
	j = 0
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
		j++
	}
	if j != len(expectedActions) {
		t.Errorf("Expected more actions: %v - found only: %d.", expectedActions, j)
	}
}

// TestRdtEffectForSanity tests for sanity.
func TestRdtEffectForSanity(t *testing.T) {
	f := newRdtActuatorFixture()
	// not much to do here, as this will "just" trigger a python script.
	state := common.State{
		Intent: struct {
			Key        string
			Priority   float64
			TargetKey  string
			TargetKind string
			Objectives map[string]float64
		}{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"p99":   20.0,
				"avail": 99.9,
			},
		},
	}
	profiles := map[string]common.Profile{
		"p99":   {ProfileType: common.ProfileTypeFromText("latency")},
		"avail": {ProfileType: common.ProfileTypeFromText("availability")},
	}
	actuator := f.newRdtTestActuator()
	// will cause a logging warning.
	actuator.Effect(&state, profiles)
}
