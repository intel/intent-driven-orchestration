package scaling

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"k8s.io/client-go/kubernetes"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	appsV1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

// dummyTracerCpu allows us to control what information we give to the actuator.
type dummyTracerCPU struct{}

func (d dummyTracerCPU) TraceEvent(_ common.State, _ common.State, _ []planner.Action) {
	klog.Fatalf("implement me")
}

func (d dummyTracerCPU) GetEffect(_ string, _ string, profileName string,
	_ int, constructor func() interface{}) (interface{}, error) {
	if profileName == "default/blurb" || profileName == "default/p95" {
		return nil, fmt.Errorf("no model was found - retuning error")
	}

	tmp := constructor().(*CPUScaleEffect)
	// the values will affect the latency and the tests results
	tmp.Popts = [][3]float64{{400, 2, 30}}
	return tmp, nil
}

// CPUScaleActuatorFixture represents a fixture for testing.
type CPUScaleActuatorFixture struct {
	test    *testing.T
	client  *fake.Clientset
	objects []runtime.Object
}

// newCPUScaleActuatorFixture initializes a new fixture for testing.
func newCPUScaleActuatorFixture(t *testing.T) *CPUScaleActuatorFixture {
	f := &CPUScaleActuatorFixture{}
	f.test = t
	return f
}

// newCPUScaleTestActuator initializes an actuator for testing.
func (f *CPUScaleActuatorFixture) newCPUScaleTestActuator(proactive bool) *CPUScaleActuator {
	f.client = fake.NewSimpleClientset(f.objects...)
	cfg := CPUScaleConfig{
		MaxProActiveCPU:            0,
		CPUMax:                     2000,
		CPUSafeGuardFactor:         0.95,
		CPURounding:                100,
		BoostFactor:                1.0,
		ProActiveLatencyPercentage: 0.8,
	}
	if proactive {
		cfg.MaxProActiveCPU = cfg.CPUMax
	}
	actuator := NewCPUScaleActuator(f.client, dummyTracerCPU{}, cfg)

	return actuator
}

// createDeployment instantiates deployment workload for testing.
func createDeployment() runtime.Object {
	return &appsV1.Deployment{
		TypeMeta: metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
		},
		Spec: appsV1.DeploymentSpec{
			Replicas: getInt32Pointer(1),
			Selector: &metaV1.LabelSelector{MatchLabels: map[string]string{
				"app": "my-function"}},
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "my-function",
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("2000m"),
									v1.ResourceMemory: {}},
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("1000m"),
									v1.ResourceMemory: {}}},
						},
						{Name: "my-function-2",
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("123m"),
									v1.ResourceMemory: {}}},
						},
					},
				},
			},
		},
	}
}

func createReplicaSet() runtime.Object {
	return &appsV1.ReplicaSet{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "my-replicaset",
			Namespace: "default",
		},
		Spec: appsV1.ReplicaSetSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "pod0123",
							Resources: v1.ResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU: resource.MustParse("200"),
								},
							},
						},
					},
				},
			},
		},
	}
}

// Tests for success.

// TestCPUScaleNextStateForSuccess tests for success.
func TestCPUScaleNextStateForSuccess(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	actuator := f.newCPUScaleTestActuator(false)
	state := common.State{Intent: common.Intent{
		Key:        "default/my-objective",
		Priority:   1.0,
		TargetKey:  "default/my-deployment",
		TargetKind: "Deployment",
		Objectives: map[string]float64{"p99": 20.0}}}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"p99": 10.0}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true}}
	actuator.NextState(&state, &goal, profiles)
}

// TestCPUScalePerformForSuccess tests for success.
func TestCPUScalePerformForSuccess(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	f.objects = append(f.objects, createDeployment())
	actuator := f.newCPUScaleTestActuator(false)
	s0 := common.State{
		Intent:      common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
		Resources: map[string]int64{
			"1_cpu_limits": 100,
		},
	}
	s0.Intent.TargetKind = "Deployment"
	plan := []planner.Action{{Name: actionName, Properties: map[string]int64{"value": 2000}}}
	actuator.Perform(&s0, plan)
}

// TestCPUScaleEffectForSuccess tests for success.
func TestCPUScaleEffectForSuccess(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	actuator := f.newCPUScaleTestActuator(false)
	s0 := common.State{}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true}}
	actuator.Effect(&s0, profiles)
}

// TestCPUScaleGetResourcesForSuccess tests for success.
func TestCPUScaleGetResourcesForSuccess(_ *testing.T) {
	s0 := common.State{Resources: map[string]int64{}}
	getResourceValues(&s0)
}

// Tests for failure.

// TestCPUScaleNextStateForFailure tests for failure.
func TestCPUScaleNextStateForFailure(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	actuator := f.newCPUScaleTestActuator(false)

	state := common.State{
		Intent: common.Intent{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"default/p99": 10.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
			},
		},
		Resources: map[string]int64{
			"1_cpu_requests": 100,
			"1_cpu_limits":   100,
		},
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{
		"default/p99":          6.0,
		"default/rps":          0.0,
		"default/availability": 0.999,
	}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
	}

	// no data in knowledge base.
	profiles["default/throughput"] = common.Profile{ProfileType: common.ProfileTypeFromText("throughput"), Minimize: false}
	profiles["default/blurb"] = common.Profile{ProfileType: common.ProfileTypeFromText("latency"), Minimize: true}
	state.Intent.Objectives["default/blurb"] = 42.0
	state.Intent.Objectives["default/throughput"] = 200.0
	states, _, _ := actuator.NextState(&state, &goal, profiles)
	if len(state.CurrentData) > 0 {
		t.Errorf("Expected empty results set as knowledge base is corrupt/empty. - got: %v", states)
	}

	// negative resource limit.
	state.Resources = map[string]int64{
		"1_cpu_limits": -100,
	}
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(state.CurrentData) > 0 {
		t.Errorf("Expected empty results - got: %v", states)
	}

	// too high resource limit
	state.Resources = map[string]int64{
		"1_cpu_limits": 100000,
	}
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(state.CurrentData) > 0 {
		t.Errorf("Expected empty results  - got: %v", states)
	}
}

// TestCPUScalePerformForFailure tests for failure.
func TestCPUScalePerformForFailure(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	f.objects = []runtime.Object{}
	actuator := f.newCPUScaleTestActuator(false)

	// deployment does not exist!
	s0 := common.State{
		Intent:      common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
		CurrentPods: map[string]common.PodState{"pod_0": {}},
	}
	plan := []planner.Action{
		{Name: actionName, Properties: map[string]int64{"value": 750}},
	}
	actuator.Perform(&s0, plan)
	expectedActions := []string{"get"}
	for i, item := range expectedActions {
		if f.client.Actions()[i].GetVerb() != item {
			t.Errorf("Expcted: %s - got: %s.", item, f.client.Actions()[i].GetVerb())
		}
	}
	if len(f.client.Actions()) != len(expectedActions) {
		t.Errorf("this should not happen - should be equal length.")
	}

	// replicaset does not exist!
	f.client.ClearActions()
	s1 := common.State{
		Intent: common.Intent{TargetKey: "default/my-rs", TargetKind: "ReplicaSet"},
	}
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]int64{"value": 750}},
	}
	actuator.Perform(&s1, plan)
	expectedActions = []string{"get"}
	for i, item := range expectedActions {
		if f.client.Actions()[i].GetVerb() != item {
			t.Errorf("Expcted: %s - got: %s.", item, f.client.Actions()[i].GetVerb())
		}
	}
	if len(f.client.Actions()) != len(expectedActions) {
		t.Errorf("this should not happen - should be equal length.")
	}

	// plan property is invalid.
	f.client.ClearActions()
	plan = []planner.Action{
		{Name: actionName, Properties: map[string]int64{"booja": 200}},
	}
	actuator.Perform(&s0, plan)
	if len(f.client.Actions()) != 0 {
		t.Errorf("This is not expected: %v", f.client.Actions())
	}
}

// TestCPUScaleGetResourcesForFailure tests for failure
func TestCPUScaleGetResourcesForFailure(t *testing.T) {
	s0 := common.State{Resources: map[string]int64{"a_cpu_limits": 100}}
	res, _ := getResourceValues(&s0)
	if res != 0 {
		t.Errorf("Should have been 0 - was: %d.", res)
	}
}

// Tests for sanity.

// TestCPUScaleNextStateForSanity tests for sanity.
func TestCPUScaleNextStateForSanity(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	actuator := f.newCPUScaleTestActuator(false)

	state := common.State{
		Intent: common.Intent{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"default/p99": 40.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod0": {
				NodeName:     "node0",
				Availability: 1.0,
				State:        "Running",
			},
		},
		Resources: map[string]int64{
			"1_cpu_limits":   1600,
			"1_cpu_requests": 1600,
		},
		CurrentData: make(map[string]map[string]float64),
	}
	goal := common.State{}
	goal.Intent.Objectives = map[string]float64{"default/p99": 50.0}
	profiles := map[string]common.Profile{
		"default/p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/p95": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/p50": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
	}

	// if we are better than goal -> do nothing.
	_, _, actions := actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should be empty, was: %v.", actions)
	}

	// if we've already looked at cpu rightsizing in this planning cycle, skip...
	goal.Intent.Objectives["default/p99"] = 30
	state.CurrentData[actionName] = map[string]float64{"actionName": 1}
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should be empty, was: %v.", actions)
	}

	// now this should work...
	state.Intent.Objectives["default/p99"] = 250
	goal.Intent.Objectives["default/p99"] = 120
	delete(state.CurrentData, actionName)
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 1 || actions[0].Properties.(map[string]int64)["value"] != 800 {
		t.Errorf("Expected one action to set 800 - got: %v", actions)
	}

	// to strict of a goal.
	goal.Intent.Objectives["default/p99"] = 1.0
	states, utilities, actions := actuator.NextState(&state, &goal, profiles)
	if len(states) != 0 || len(utilities) != 0 || len(actions) != 0 {
		t.Errorf("Resultsets should be empty: %v, %v, %v.", states, utilities, actions)
	}

	// proactive enabled
	actuator = f.newCPUScaleTestActuator(true)
	goal.Intent.Objectives["default/p99"] = 20
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 1 || actions[0].Properties.(map[string]int64)["proactive"] != 1 || actions[0].Properties.(map[string]int64)["value"] != 1800 {
		t.Errorf("Should contain 1 proactive action; was: %v", actions)
	}

	// we've already done proactive scale out.
	state.CurrentPods["proactiveResourceAlloc"] = common.PodState{}
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should have no actions: %v", actions)
	}

	// maxProactive reached.
	delete(state.CurrentPods, "proactiveResourceAlloc")
	state.Resources = map[string]int64{
		"1_cpu_requests": actuator.cfg.MaxProActiveCPU,
	}
	states, utilities, actions = actuator.NextState(&state, &goal, profiles)
	if len(states) != 0 || len(utilities) != 0 || len(actions) != 0 {
		t.Errorf("Resultsets should be empty: %v, %v, %v.", states, utilities, actions)
	}

	// proActive should decrement; For p95 there is no model and resources are set to high...
	delete(state.Intent.Objectives, "default/p99")
	delete(goal.Intent.Objectives, "default/p99")
	state.Intent.Objectives["default/p95"] = 100
	goal.Intent.Objectives["default/p95"] = 200
	_, _, actions = actuator.NextState(&state, &goal, profiles)
	if len(actions) != 1 || actions[0].Properties.(map[string]int64)["proactive"] != 1 || actions[0].Properties.(map[string]int64)["value"] < 1800 || actions[0].Properties.(map[string]int64)["value"] > 1900 {
		t.Errorf("Should contain 1 proactive action; was: %v", actions)
	}

	// ensure we get as many states back as we have objectives.
	delete(state.Intent.Objectives, "default/p95")
	delete(goal.Intent.Objectives, "default/p95")
	state.Resources["1_cpu_limits"] = 1600
	state.Resources["1_cpu_requests"] = 1600
	state.Intent.Objectives["default/p99"] = 50
	state.Intent.Objectives["default/p50"] = 50
	goal.Intent.Objectives["default/p99"] = 40
	goal.Intent.Objectives["default/p50"] = 40
	klog.Infof("Current %+v, Goal %+v", state, goal)
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(states) != 2 {
		t.Errorf("Should have returned 2 states; was: %v", states)
	}

	// ensure an "empty" state does not crash the actuator.
	actuator = f.newCPUScaleTestActuator(false)
	delete(goal.Intent.Objectives, "default/p95")
	goal.Intent.Objectives["default/p99"] = 120
	emptyState := common.State{
		Intent: common.Intent{
			Key:        "default/my-objective",
			Priority:   1.0,
			TargetKey:  "default/my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{"default/p99": 250.0},
		},
	}
	_, _, actions = actuator.NextState(&emptyState, &goal, profiles)
	if len(actions) != 0 {
		t.Errorf("Should contain no action; was: %v", actions)
	}

	// test boost factor - QoS should be set accordingly.
	actuator = f.newCPUScaleTestActuator(false)
	actuator.cfg.BoostFactor = 0.8
	delete(state.Intent.Objectives, "default/p95")
	state.Resources = nil
	state.Intent.Objectives["default/p99"] = 200
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(states) != 2 {
		t.Errorf("Should have return at least 2 states - was %v", states)
	}
	for name, pod := range states[0].CurrentPods {
		if pod.QoSClass != "BestEffort" {
			t.Errorf("Expected POD %s to be in besteffort QoS - was: %s.", name, pod.QoSClass)
		}
	}

	// boost factor >= 1.0
	actuator = f.newCPUScaleTestActuator(false)
	actuator.cfg.BoostFactor = 2.0
	states, _, _ = actuator.NextState(&state, &goal, profiles)
	if len(states) != 2 { // 2 objectives.
		t.Errorf("Should have return at least 2 state - was %v", states)
	}
	for name, pod := range states[0].CurrentPods {
		if pod.QoSClass != "Burstable" {
			t.Errorf("Expected POD %s to be in burstable QoS - was: %s.", name, pod.QoSClass)
		}
	}
}

// TestCPUScalePerformForSanity tests for sanity.
func TestCPUScalePerformForSanity(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	f.objects = append(f.objects, createDeployment())
	actuator := f.newCPUScaleTestActuator(false)

	// test for deployment.
	s0 := common.State{
		Intent: common.Intent{TargetKey: "default/my-deployment", TargetKind: "Deployment"},
	}
	plan := []planner.Action{
		{Name: actionName, Properties: map[string]int64{"value": 1000}},
	}
	actuator.Perform(&s0, plan)
	expectedActions := []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
	if len(expectedActions) != len(f.client.Actions()) {
		t.Errorf("Expecting action list to be equal length: %v, %v", expectedActions, f.client.Actions())
	}
	updatedObject, _ := f.client.AppsV1().Deployments("default").Get(context.TODO(), "my-deployment", metaV1.GetOptions{})
	res := updatedObject.Spec.Template.Spec.Containers
	val, ok := res[1].Resources.Requests["cpu"]
	if val.MilliValue() != 1000 || !ok {
		t.Errorf("Requests should have been 1000; was: %v", res[1].Resources.Requests["cpu"])
	}
	val, ok = res[1].Resources.Limits["cpu"]
	if val.MilliValue() != 1000 || !ok {
		t.Errorf("Limits should have been 1000; was: %v", res[1].Resources.Limits["cpu"])
	}

	// test for replicaset.
	f.client.ClearActions()
	f.objects = []runtime.Object{createReplicaSet()}
	actuator = f.newCPUScaleTestActuator(false)
	s0.Intent.TargetKey = "default/my-replicaset"
	s0.Intent.TargetKind = "ReplicaSet"
	actuator.Perform(&s0, plan)
	expectedActions = []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
	if len(expectedActions) != len(f.client.Actions()) {
		t.Errorf("Expecting action list to be equal length: %v, %v", expectedActions, f.client.Actions())
	}
	updatedRS, _ := f.client.AppsV1().ReplicaSets("default").Get(context.TODO(), "my-replicaset", metaV1.GetOptions{})
	res = updatedRS.Spec.Template.Spec.Containers
	val, ok = res[0].Resources.Requests["cpu"]
	if val.MilliValue() != 1000 || !ok {
		t.Errorf("Requests should have been 1000; was: %v", res[0].Resources.Requests["cpu"])
	}
	val, ok = res[0].Resources.Limits["cpu"]
	if val.MilliValue() != 1000 || !ok {
		t.Errorf("Limits should have been 1000; was: %v", res[0].Resources.Limits["cpu"])
	}

	// Test boost factor.
	f.client.ClearActions()
	f.objects = []runtime.Object{createReplicaSet()}
	actuator = f.newCPUScaleTestActuator(false)
	actuator.Perform(&s0, plan)
	expectedActions = []string{"get", "update"}
	for i, action := range f.client.Actions() {
		if action.GetVerb() != expectedActions[i] {
			t.Errorf("Expected %s - got %s.", expectedActions[i], action)
		}
	}
	if len(expectedActions) != len(f.client.Actions()) {
		t.Errorf("Expecting action list to be equal length: %v, %v", expectedActions, f.client.Actions())
	}
	updatedRS, _ = f.client.AppsV1().ReplicaSets("default").Get(context.TODO(), "my-replicaset", metaV1.GetOptions{})
	res = updatedRS.Spec.Template.Spec.Containers
	val, ok = res[0].Resources.Requests["cpu"]
	if val.MilliValue() != 1000 || !ok {
		t.Errorf("Requests should have been 1000; was: %v", res[0].Resources.Requests["cpu"])
	}
	val, ok = res[0].Resources.Limits["cpu"]
	if val.MilliValue() != 1000 || !ok {
		t.Errorf("Limits should have been 1000; was: %v", res[0].Resources.Limits["cpu"])
	}

	// Boost factor < 1.0 should lead to no limits being set.
	f.client.ClearActions()
	f.objects = []runtime.Object{createReplicaSet()}
	actuator = f.newCPUScaleTestActuator(false)
	actuator.cfg.BoostFactor = 0.9
	actuator.Perform(&s0, plan)
	updatedRS, _ = f.client.AppsV1().ReplicaSets("default").Get(context.TODO(), "my-replicaset", metaV1.GetOptions{})
	res = updatedRS.Spec.Template.Spec.Containers
	val, ok = res[0].Resources.Limits["cpu"]
	if ok {
		t.Errorf("Limits should not have been set; was: %v", val)
	}

	// Boost factor > 1.0 should lead to POD being in Burstable QoS.
	f.client.ClearActions()
	f.objects = []runtime.Object{createReplicaSet()}
	actuator = f.newCPUScaleTestActuator(false)
	actuator.cfg.BoostFactor = 2.0
	actuator.Perform(&s0, plan)
	updatedRS, _ = f.client.AppsV1().ReplicaSets("default").Get(context.TODO(), "my-replicaset", metaV1.GetOptions{})
	res = updatedRS.Spec.Template.Spec.Containers
	val, ok = res[0].Resources.Limits["cpu"]
	if val.MilliValue() != 2000 || !ok {
		t.Errorf("Limits should have been 2000; was: %v", res[0].Resources.Limits["cpu"])
	}
}

// TestCPUScaleEffectForSanity tests for sanity.
func TestCPUScaleEffectForSanity(t *testing.T) {
	f := newCPUScaleActuatorFixture(t)
	// this will "just" trigger a python script.
	state := common.State{Intent: common.Intent{Key: "default/my-objective", Priority: 1.0, TargetKey: "default/my-deployment",
		TargetKind: "Deployment", Objectives: map[string]float64{"p99": 20.0}}}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true}}
	actuator := f.newCPUScaleTestActuator(false)
	actuator.Effect(&state, profiles)

	// check with None.
	actuator.cfg.Script = "None"
	actuator.Effect(&state, profiles)
}

// TestCPUScaleGetResourcesForSuccess tests for sanity.
func TestCPUScaleGetResourcesForSanity(t *testing.T) {
	s0 := common.State{Resources: map[string]int64{}}
	res, _ := getResourceValues(&s0)
	if res != 0 {
		t.Errorf("Should have been 0 - was: %v", res)
	}
	// request defined.
	s0.Resources["0_cpu_requests"] = 200
	res, _ = getResourceValues(&s0)
	if res != 200 {
		t.Errorf("Should have been 200 - was: %v", res)
	}
	// limits defined.
	s0.Resources["0_cpu_limits"] = 400
	res, _ = getResourceValues(&s0)
	if res != 400 {
		t.Errorf("Should have been 400 - was: %v", res)
	}
	// the last container matters.
	s0.Resources["1_cpu_requests"] = 100
	res, containerIndex := getResourceValues(&s0)
	if res != 100 || containerIndex != 1 {
		t.Errorf("Should have been 100 - was: %v", res)
	}
}

func TestCPUScaleActuator_NextState(t *testing.T) {
	type fields struct {
		cfg    CPUScaleConfig
		tracer controller.Tracer
		apps   kubernetes.Interface
	}

	type args struct {
		state    common.State
		goal     common.State
		profiles map[string]common.Profile
	}

	f := newCPUScaleActuatorFixture(t)
	actuator := f.newCPUScaleTestActuator(true)

	newState := common.State{
		Intent: common.Intent{
			Key:        "default/my-function-intent",
			Priority:   1.0,
			TargetKey:  "default/function-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"default/p95latency":   770,
				"default/availability": 0.95},
		},
		CurrentPods: map[string]common.PodState{
			"somePod-0x1-0y1": {
				Availability: 1.0,
			},
		},
		CurrentData: map[string]map[string]float64{"cpu_usage": {}},
		Resources: map[string]int64{
			"1_cpu_limits":   1000,
			"1_cpu_requests": 1000,
		},
		Annotations: nil,
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []common.State
		want1  []float64
		want2  []planner.Action
	}{
		{
			name: "tc-1",
			fields: fields{
				cfg:    actuator.cfg,
				tracer: actuator.tracer,
				apps:   actuator.apps,
			},
			args: args{
				state: common.State{
					Intent: common.Intent{
						Key:        "default/my-function-intent",
						Priority:   1.0,
						TargetKey:  "default/function-deployment",
						TargetKind: "Deployment",
						Objectives: map[string]float64{
							"default/p95latency":   786.282,
							"default/availability": 1},
					},
					CurrentPods: map[string]common.PodState{
						"somePod-0x1-0y1": {
							NodeName:     "node4",
							Availability: 1,
							State:        "Running",
							QoSClass:     "Burstable",
						},
					},
					CurrentData: map[string]map[string]float64{"cpu_usage": {}},
					Resources: map[string]int64{
						"1_cpu_limits":   500,
						"1_cpu_requests": 500,
					},
					Annotations: nil,
				},
				goal: newState,
				profiles: map[string]common.Profile{
					"default/p95latency":   {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
					"default/availability": {ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
				},
			},

			want:  []common.State{newState},
			want1: []float64{0.5},
			want2: []planner.Action{{Name: actionName,
				Properties: map[string]int64{"value": 1000}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := CPUScaleActuator{
				cfg:    tt.fields.cfg,
				tracer: tt.fields.tracer,
				apps:   tt.fields.apps,
			}
			newState.Intent.Objectives["default/p95latency"] = cs.predictLatency(
				[3]float64{400, 2, 30}, 900)
			newState.Intent.Objectives["default/availability"] = 1
			got, got1, got2 := cs.NextState(&tt.args.state, &tt.args.goal, tt.args.profiles) //#nosec G601 -- NA as this is a test.

			if !reflect.DeepEqual(got[0].Resources, tt.want[0].Resources) {
				t.Errorf("NextState() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("NextState() got1 = %v, want %v", got1, tt.want1)
			}
			if !reflect.DeepEqual(got2, tt.want2) {
				t.Errorf("NextState() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func TestCPUScaleActuator_predictLatencyCPU(t *testing.T) {
	type fields struct {
		cfg    CPUScaleConfig
		tracer controller.Tracer
		apps   kubernetes.Interface
	}

	type args struct {
		popt   []float64
		limCPU int64
	}

	f := newCPUScaleActuatorFixture(t)
	actuator := f.newCPUScaleTestActuator(false)

	tests := []struct {
		name   string
		fields fields
		args   args
		want   float64
	}{
		{name: "test", args: args{
			popt:   []float64{1, 1, 100},
			limCPU: 2000},
			fields: fields{
				cfg:    actuator.cfg,
				tracer: actuator.tracer,
				apps:   actuator.apps,
			},
			want: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actuator.predictLatency([3]float64(tt.args.popt), float64(tt.args.limCPU)); math.Round(got) != tt.want {
				t.Errorf("predictLatency() = %v, want %v", got, tt.want)
			}
		})
	}
}
