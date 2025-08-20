package platform

import (
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	appsV1 "k8s.io/api/apps/v1"
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
		Analytics:   "analytics/test_analyze.py",
		Prediction:  "analytics/test_predict.py",
		Options:     []string{"None", "option_a", "option_b", "option_c"},
	}
	actuator := NewRdtActuator(f.client, dummyTracer{}, cfg)
	return actuator
}

func (f *rdtActuatorFixture) cleanUp(actuator *RdtActuator) {
	klog.Infof("Going to kill process with PID: %d.", actuator.Cmd.Process.Pid)
	err := actuator.Cmd.Process.Kill()
	if err != nil {
		klog.Fatalf("Could not terminate prediction service: %s", err)
	}
}

// Tests for success.

// TestRdtNextStateForSuccess tests for success.
func TestRdtNextStateForSuccess(_ *testing.T) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()
	defer f.cleanUp(actuator)

	state := common.State{
		Intent: common.Intent{
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
func TestRdtPerformForSuccess(_ *testing.T) {
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
	defer f.cleanUp(actuator)
	state := common.State{
		Intent:      common.Intent{TargetKey: "default/my-function", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	var plan []planner.Action
	actuator.Perform(&state, plan)
}

// TestRdtEffectForSuccess tests for success.
func TestRdtEffectForSuccess(_ *testing.T) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()
	defer f.cleanUp(actuator)
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
	defer f.cleanUp(actuator)
	state := common.State{
		Intent: common.Intent{
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
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

	// test same for replicaset
	f.client.ClearActions()
	state.Intent.TargetKind = "ReplicaSet"
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 1 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}
}

// TestRdtEffectForFailure tests for failure.
func TestRdtEffectForFailure(_ *testing.T) {
	f := newRdtActuatorFixture()
	// not much to do here, as this will "just" trigger a python script.
	state := common.State{
		Intent: common.Intent{
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
	defer f.cleanUp(actuator)
	actuator.config.Analytics = "foobar"
	// will cause a logging warning.
	actuator.Effect(&state, profiles)
}

// Tests for sanity.

func TestGetResourcesForSanity(t *testing.T) {
	var tests = []struct {
		name   string
		input  map[string]int64
		output int64
	}{
		{
			name:   "should_work",
			input:  map[string]int64{"0_cpu_limits": 1000},
			output: 1000,
		},
		{
			name:   "only_requests",
			input:  map[string]int64{"0_cpu_requests": 1000},
			output: 0,
		},
		{
			name:   "multiple_containers",
			input:  map[string]int64{"1_cpu_limits": 1000, "2_cpu_limits": 2000, "0_cpu_limits": 500},
			output: 2000,
		},
		{
			name:   "no_cpu_info",
			input:  map[string]int64{"0_gpu_limits": 1},
			output: 0,
		},
		{
			name:   "faulty format",
			input:  map[string]int64{"a_b_c": 1},
			output: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := getResources(tt.input)
			if val != tt.output {
				t.Errorf("Expected %d - got %d for test %s.", tt.output, val, tt.name)
			}
		})
	}
}

// TestRdtNextStateForSanity tests for sanity.
func TestRdtNextStateForSanity(t *testing.T) {
	f := newRdtActuatorFixture()
	actuator := f.newRdtTestActuator()
	defer f.cleanUp(actuator)

	state := common.State{
		Intent: common.Intent{
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
			"pod_0": {
				QoSClass: "Guaranteed",
			},
		},
		CurrentData: map[string]map[string]float64{
			"cpu_value": {"node": 10.0},
		},
		Annotations: map[string]string{"foo": "bar"},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	goal.Intent.Objectives = map[string]float64{
		"default/p99":   5.0,
		"default/blurb": 10.0,
	}
	profiles := map[string]common.Profile{
		"default/p95":   {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/p99":   {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/blurb": {ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
	}

	states, utils, actions := actuator.NextState(&state, &goal, profiles)
	if len(states) != 1 || len(utils) != 1 || len(actions) != 1 {
		t.Errorf("Expected one entry each: %v, %v, %v.", states, utils, actions)
	}
	// check if annotations are set.
	if _, found := states[0].Annotations["rdtVisited"]; !found {
		t.Errorf("Expected the temp blocker in annotation:  %v.", states[0].Annotations)
	}
	// check if the resulting action matches.
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
	state.Annotations[actuator.config.AnnotationName] = "option_b"
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
	state.Annotations["rdtVisited"] = ""
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) > 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// we do not expect to set an option if it is already set...
	state.Annotations[actuator.config.AnnotationName] = "option_a"
	delete(state.Annotations, "rdtVisited")
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// expect empty result if script return -1.0
	delete(state.Annotations, "rdtVisited")
	delete(state.Annotations, actuator.config.AnnotationName)
	delete(state.Intent.Objectives, "default/p99")
	delete(goal.Intent.Objectives, "default/p99")
	state.Intent.Objectives["default/p72"] = 10.0
	goal.Intent.Objectives["default/p72"] = 3.0
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Expected no actions - got: %v.", actions)
	}

	// check if we have multiple results when we have multiple targeted objectives.
	delete(state.Intent.Objectives, "default/p72")
	delete(state.Annotations, "rdtVisited")
	delete(state.Annotations, actuator.config.AnnotationName)
	state.Intent.Objectives["default/p99"] = 20
	state.Intent.Objectives["default/p95"] = 15
	goal.Intent.Objectives["default/p99"] = 5.0
	goal.Intent.Objectives["default/p95"] = 7.5
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions - got: %v.", actions)
	}

	// POD not in guaranteed class.
	delete(state.Annotations, "rdtVisited")
	delete(state.Intent.Objectives, "default/p72")
	delete(state.CurrentPods, "pod_0")
	state.CurrentPods["pod_1"] = common.PodState{
		QoSClass: "Burstable",
	}
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
	defer f.cleanUp(actuator)

	state := common.State{
		Intent: common.Intent{
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
			"pod_0": {
				QoSClass: "Guaranteed",
			},
		},
		CurrentData: map[string]map[string]float64{
			"cpu_value": {"node": 10.0},
		},
		Annotations: map[string]string{"foo": "bar"},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	goal.Intent.Objectives = map[string]float64{
		"default/p99":   5.0,
		"default/blurb": 10.0,
	}
	profiles := map[string]common.Profile{
		"default/p99":   {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/blurb": {ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
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
		&appsV1.Deployment{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-function",
				Namespace: "default",
			},
		},
		&appsV1.ReplicaSet{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-function",
				Namespace: "default",
			},
		},
	}
	actuator := f.newRdtTestActuator()
	defer f.cleanUp(actuator)
	state := common.State{
		Intent: common.Intent{
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
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

	// check if actions are also called for replicaset.
	f.client.ClearActions()
	state.Intent.TargetKind = "ReplicaSet"
	plan = []planner.Action{
		{Name: rdtActionName, Properties: map[string]string{"option": "option_a"}},
	}
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

	// check if annotation is removed if option is None:
	f.client.ClearActions()
	plan = []planner.Action{
		{Name: rdtActionName, Properties: map[string]string{"option": "None"}},
	}
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
func TestRdtEffectForSanity(_ *testing.T) {
	f := newRdtActuatorFixture()
	// not much to do here, as this will "just" trigger a python script.
	state := common.State{
		Intent: common.Intent{
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
		"p99":   {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"avail": {ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
	}
	actuator := f.newRdtTestActuator()
	defer f.cleanUp(actuator)
	// will cause a logging warning.
	actuator.Effect(&state, profiles)
}
