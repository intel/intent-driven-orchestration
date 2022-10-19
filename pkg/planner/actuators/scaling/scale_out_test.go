package scaling

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	appsV1 "k8s.io/api/apps/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

// dummyTracer allows us to control what information we give to the actuator.
type dummyTracer struct{}

func (d dummyTracer) TraceEvent(_ common.State, _ common.State, _ []planner.Action) {
	klog.Fatalf("implement me")
}

func (d dummyTracer) GetEffect(_ string, _ string, profileName string, _ int, constructor func() interface{}) (interface{}, error) {
	if profileName == "default/blurb" {
		return nil, fmt.Errorf("nothing here")
	}
	tmp := constructor().(*ScaleOutEffect)
	tmp.ReplicaRange = []int{1, 7}
	tmp.Popt = []float64{3.75, 0.005, 3.0}
	return tmp, nil
}

// scaleOutActuatorFixture represents a fixture for testing.
type scaleOutActuatorFixture struct {
	test    *testing.T
	client  *fake.Clientset
	objects []runtime.Object
}

// newScaleOutActuatorFixture initializes a new fixture for testing.
func newScaleOutActuatorFixture(t *testing.T) *scaleOutActuatorFixture {
	f := &scaleOutActuatorFixture{}
	f.test = t
	return f
}

// newScaleOutTestActuator initializes an actuator for testing.
func (f *scaleOutActuatorFixture) newScaleOutTestActuator() *ScaleOutActuator {
	f.client = fake.NewSimpleClientset(f.objects...)
	cfg := ScaleOutConfig{
		MaxProActiveScaleOut: 4,
		MaxPods:              128,
	}
	actuator := NewScaleOutActuator(f.client, dummyTracer{}, cfg)
	return actuator
}

// Tests for success.

// TestScaleNextStateForSuccess tests for success.
func TestScaleNextStateForSuccess(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	actuator := f.newScaleOutTestActuator()
	state := common.State{Intent: struct {
		Key        string
		Priority   float64
		TargetKey  string
		TargetKind string
		Objectives map[string]float64
	}{Key: "default/my-objective", Priority: 1.0, TargetKey: "default/my-deployment", TargetKind: "Deployment", Objectives: map[string]float64{"p99": 20.0}}}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"p99": 10.0}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}}
	actuator.NextState(&state, &goal, profiles)
}

// TestScalePerformForSuccess tests for success.
func TestScalePerformForSuccess(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	f.objects = []runtime.Object{
		&appsV1.Deployment{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-deployment",
				Namespace: "default",
			},
			Spec: appsV1.DeploymentSpec{
				Replicas: getInt32Pointer(1),
			},
		},
	}
	actuator := f.newScaleOutTestActuator()
	s0 := common.State{
		Intent:      common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	var plan []planner.Action
	actuator.Perform(&s0, plan)
}

// TestScaleEffectForSuccess tests for success.
func TestScaleEffectForSuccess(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	state := common.State{Intent: struct {
		Key        string
		Priority   float64
		TargetKey  string
		TargetKind string
		Objectives map[string]float64
	}{Key: "default/my-objective", Priority: 1.0, TargetKey: "default/my-deployment", TargetKind: "Deployment", Objectives: map[string]float64{"p99": 20.0, "throughput": 10}}}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}, "throughput": {ProfileType: common.ProfileTypeFromText("throughput")}}
	actuator := f.newScaleOutTestActuator()
	actuator.Effect(&state, profiles)
}

// Tests for failure.

// TestScaleNextStateForFailure tests for failure.
func TestScaleNextStateForFailure(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	actuator := f.newScaleOutTestActuator()

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
			Objectives: map[string]float64{"default/p99": 10.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {NodeName: "node0", Availability: 1.0},
		},
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"default/p99": 3.0, "default/rps": 0.0, "default/availability": 0.999}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency")},
	}

	// no throughput is being tracked.
	states, _, _ := actuator.NextState(&state, &goal, profiles)
	if len(states) > 0 {
		t.Errorf("Expected empty results set as we've not defined a throughput obejctive - got: %v", states)
	}

	// no data in knowledge base.
	profiles["default/throughput"] = common.Profile{ProfileType: common.ProfileTypeFromText("throughput")}
	profiles["default/blurb"] = common.Profile{ProfileType: common.ProfileTypeFromText("latency")}
	state.Intent.Objectives["default/blurb"] = 42.0
	state.Intent.Objectives["default/throughput"] = 200.0
	// adding pods to proActive action does not kick in.
	for i := 0; i < actuator.cfg.MaxProActiveScaleOut; i++ {
		state.CurrentPods["pod"+strconv.Itoa(i)] = common.PodState{Availability: 1.0}
	}
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(states) > 0 {
		t.Errorf("Expected empty results set as knowledge base is corrupt/empty. - got: %v", states)
	}
}

// TestScalePerformForFailure tests for failure.
func TestScalePerformForFailure(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	f.objects = []runtime.Object{}
	actuator := f.newScaleOutTestActuator()

	// deployment does not exist!
	s0 := common.State{
		Intent:      common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	plan := []planner.Action{
		{Name: scaleOutActionName, Properties: map[string]int32{"factor": 2}},
		{Name: rmPodActionName},
	}
	actuator.Perform(&s0, plan)
	if len(f.client.Actions()) != 1 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}

	// replicaSet does not exist!
	actuator = f.newScaleOutTestActuator()
	s0.Intent.TargetKey = "default/my-replicaset"
	s0.Intent.TargetKind = "ReplicaSet"
	actuator.Perform(&s0, plan)
	if len(f.client.Actions()) != 1 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}
}

// TestScaleEffectForFailure tests for failure.
func TestScaleEffectForFailure(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	actuator := f.newScaleOutTestActuator()
	state := common.State{Intent: struct {
		Key        string
		Priority   float64
		TargetKey  string
		TargetKind string
		Objectives map[string]float64
	}{Key: "default/my-objective", Priority: 1.0, TargetKey: "default/my-deployment", TargetKind: "Deployment", Objectives: map[string]float64{"p99": 20.0, "throughput": 10}}}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}, "throughput": {ProfileType: common.ProfileTypeFromText("throughput")}}

	actuator.cfg.Script = "abc.xyz"
	actuator.Effect(&state, profiles)
}

// Tests for sanity.

// TestScaleNextStateForSanity tests for sanity.
func TestScaleNextStateForSanity(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	actuator := f.newScaleOutTestActuator()

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
			Objectives: map[string]float64{"default/p99": 10.0, "default/rps": 100.0, "default/availability": 1.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {NodeName: "node0", Availability: 1.0},
		},
	}
	goal := common.State{}
	goal.Intent.Priority = 1.0
	goal.Intent.Objectives = map[string]float64{"default/p99": 3.0, "default/rps": 0.0, "default/availability": 0.999}
	profiles := map[string]common.Profile{
		"default/p99":          {ProfileType: common.ProfileTypeFromText("latency")},
		"default/rps":          {ProfileType: common.ProfileTypeFromText("throughput")},
		"default/availability": {ProfileType: common.ProfileTypeFromText("availability")},
	}

	states, utilities, actions := actuator.NextState(&state, &goal, profiles)
	if len(states) < 1 || len(utilities) < 1 || len(actions) < 1 {
		t.Errorf("Resultsets are empty: %v, %v, %v.", states, utilities, actions)
	}
	// check if results match for scale-out
	if len(states[0].CurrentPods) != 3 || actions[0].Name != actuator.Name() || actions[0].Properties.(map[string]int32)["factor"] != 2 {
		t.Errorf("Expected a scale out by factor of 2 - got: %v.", actions[0])
	}
	if utilities[0] > 0.95 {
		t.Errorf("Expected utiltiy to be < 1.0 - got: %v.", utilities)
	}

	// empty results if no solution can be found.
	goal.Intent.Objectives["default/p99"] = 0.001
	// adding pods to proActive action does not kick in.
	for i := 0; i < actuator.cfg.MaxProActiveScaleOut; i++ {
		state.CurrentPods["pod"+strconv.Itoa(i)] = common.PodState{Availability: 1.0}
	}
	states, utilities, actions = actuator.NextState(&state, &goal, profiles)
	if len(states) != 0 || len(utilities) != 0 || len(actions) != 0 {
		t.Errorf("Resultsets should be empty: %v, %v, %v.", states, utilities, actions)
	}

	// empty result if we're good for now.
	goal.Intent.Objectives["default/p99"] = 10.0
	states, utilities, actions = actuator.NextState(&state, &goal, profiles)
	if len(states) != 0 || len(utilities) != 0 || len(actions) != 0 {
		t.Errorf("Resultsets should be empty: %v, %v, %v.", states, utilities, actions)
	}

	// no solution can be found, although we've a "good" model.
	goal.Intent.Objectives["default/p99"] = 0.001
	actuator.cfg.MaxPods = 2
	states, utilities, actions = actuator.NextState(&state, &goal, profiles)
	if len(states) != 0 || len(utilities) != 0 || len(actions) != 0 {
		t.Errorf("Resultsets should be empty: %v, %v, %v.", states, utilities, actions)
	}

	// testing proActive action, removing some pods.
	for i := 0; i < actuator.cfg.MaxProActiveScaleOut; i++ {
		delete(state.CurrentPods, "pod"+strconv.Itoa(i))
	}
	states, utilities, actions = actuator.NextState(&state, &goal, profiles)
	if len(states) != 1 || len(utilities) != 1 || len(actions) != 1 {
		t.Errorf("Resultsets should not be empty: %v, %v, %v.", states, utilities, actions)
	}
	if actions[0].Properties.(map[string]int32)["proactive"] != 1 || utilities[0] != 0.1 {
		t.Errorf("Action should be marked as being proactive, utiltiy == 0.01 -got %v, %v.", states[0], utilities[0])
	}
}

// TestScalePerformForSanity tests for sanity.
func TestScalePerformForSanity(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	f.objects = []runtime.Object{
		&appsV1.Deployment{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-deployment",
				Namespace: "default",
			},
			Spec: appsV1.DeploymentSpec{
				Replicas: getInt32Pointer(1),
			},
		},
		&appsV1.ReplicaSet{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-replicaset",
				Namespace: "default",
			},
			Spec: appsV1.ReplicaSetSpec{
				Replicas: getInt32Pointer(1),
			},
		},
	}
	actuator := f.newScaleOutTestActuator()

	// test for deployment.
	s0 := common.State{
		Intent:      common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	plan := []planner.Action{
		{Name: scaleOutActionName, Properties: map[string]int32{"factor": 2}},
		{Name: rmPodActionName},
	}
	actuator.Perform(&s0, plan)
	expectedActions := []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}

	// test for replicaset.
	actuator = f.newScaleOutTestActuator()
	s0.Intent.TargetKey = "default/my-replicaset"
	s0.Intent.TargetKind = "ReplicaSet"
	actuator.Perform(&s0, plan)
	expectedActions = []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
}

// TestScaleEffectForSanity tests for sanity.
func TestScaleEffectForSanity(t *testing.T) {
	f := newScaleOutActuatorFixture(t)
	// not much to do here, as this will "just" trigger a python script.
	state := common.State{Intent: struct {
		Key        string
		Priority   float64
		TargetKey  string
		TargetKind string
		Objectives map[string]float64
	}{Key: "default/my-objective", Priority: 1.0, TargetKey: "default/my-deployment", TargetKind: "Deployment", Objectives: map[string]float64{"p99": 20.0}}}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}}
	actuator := f.newScaleOutTestActuator()
	// will cause a logging warning.
	actuator.Effect(&state, profiles)

	// adding the throughput based objective profile will help :-)
	state.Intent.Objectives["default/rps"] = 100
	profiles["default/rps"] = common.Profile{ProfileType: common.ProfileTypeFromText("throughput")}
	actuator.Effect(&state, profiles)
}
