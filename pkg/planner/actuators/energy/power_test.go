package energy

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/klog/v2"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	appsV1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// powerActuatorFixture represents a fixture for testing.
type powerActuatorFixture struct {
	test    *testing.T
	client  *fake.Clientset
	objects []runtime.Object
}

// newPowerActuatorFixture initializes a new fixture for testing.
func newPowerActuatorFixture(t *testing.T) *powerActuatorFixture {
	f := &powerActuatorFixture{}
	f.test = t
	return f
}

func (f *powerActuatorFixture) newPowerTestActuator(enabled bool) *PowerActuator {
	f.client = fake.NewSimpleClientset(f.objects...)
	cfg := PowerActuatorConfig{
		PythonInterpreter:   "python3",
		Prediction:          "analytics/test_predict.py",
		Analytics:           "analytics/test_analytics.py",
		PowerProfiles:       []string{"None", "power.intel.com/balance-power", "power.intel.com/balance-performance", "power.intel.com/performance"},
		ProactiveCandidates: enabled,
		RenewableLimit:      0.75,
		StepDown:            1,
	}
	return NewPowerActuator(f.client, nil, cfg)
}

func (f *powerActuatorFixture) createDeploymentObject(name string) runtime.Object {
	deployment := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: appsV1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Resources: v1.ResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									"cpus":                          resource.MustParse("1"),
									"power.intel.com/balance-power": resource.MustParse("1"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									"cpus":                          resource.MustParse("1"),
									"power.intel.com/balance-power": resource.MustParse("1"),
								},
							},
						},
						{
							Resources: v1.ResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									"cpus":                          resource.MustParse("4"),
									"power.intel.com/balance-power": resource.MustParse("4"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									"cpus":                          resource.MustParse("4"),
									"power.intel.com/balance-power": resource.MustParse("4"),
								},
							},
						},
					},
				},
			},
		},
	}
	return deployment
}

func (f *powerActuatorFixture) cleanUp(actuator *PowerActuator) {
	err := actuator.Cmd.Process.Kill()
	if err != nil {
		klog.Fatalf("Could not terminate prediction service: %s", err)
	}
}

// Tests for success.

// TestPowerNextStateForSuccess tests for success.
func TestPowerNextStateForSuccess(t *testing.T) {
	f := newPowerActuatorFixture(t)
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	start := common.State{
		Intent: common.Intent{
			Key:        "default/my-app-intent",
			Priority:   1.0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{"default/p99latency": 280.0},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {QoSClass: "Guaranteed"},
		},
		Resources: map[string]int64{"cpus": 1000},
	}
	goal := common.State{
		Intent: common.Intent{Objectives: map[string]float64{"default/p99latency": 60.0}},
	}
	profiles := map[string]common.Profile{"default/p99latency": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true}}
	states, _, _ := actuator.NextState(&start, &goal, profiles)
	if len(states) < 1 {
		t.Errorf("Now this isn't right - should have a result: %v", states)
	}
}

// TestPowerPerformForSuccess tests for success.
func TestPowerPerformForSuccess(t *testing.T) {
	f := newPowerActuatorFixture(t)
	f.objects = []runtime.Object{f.createDeploymentObject("my-function")}
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	state := common.State{
		Intent: common.Intent{TargetKey: "default/my-function", TargetKind: "Deployment"},
	}
	var plan []planner.Action
	actuator.Perform(&state, plan)
}

// TestPowerEffectForSuccess tests for success.
func TestPowerEffectForSuccess(t *testing.T) {
	f := newPowerActuatorFixture(t)
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	state := common.State{
		Intent: common.Intent{
			Objectives: map[string]float64{"default/p99latency": 100.0},
		},
	}
	profiles := map[string]common.Profile{"default/p99latency": {ProfileType: common.ProfileTypeFromText("latency")}}
	actuator.Effect(&state, profiles)
}

// Tests for failure.

// TestPowerPerformForFailure tests for failure.
func TestPowerPerformForFailure(t *testing.T) {
	f := newPowerActuatorFixture(t)
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	plan := []planner.Action{{Name: actuator.Name(), Properties: map[string]string{"profile": "power.intel.com/performance"}}}
	state := common.State{
		Intent: common.Intent{
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
		},
	}

	// non-existing deployment...
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 1 && f.client.Actions()[0].GetVerb() != "get" {
		t.Errorf("This is not expected - should only have seen 1 get...: %v", f.client.Actions())
	}
}

// TestPowerEffectForFailure tests for failure.
func TestPowerEffectForFailure(t *testing.T) {
	f := newPowerActuatorFixture(t)
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	actuator.config.Analytics = "foo.py"
	state := common.State{}
	profiles := map[string]common.Profile{}
	actuator.Effect(&state, profiles)
}

// Tests for sanity.

// TestPowerFindProfileForSanity tests for sanity.
func TestPowerFindProfileForSanity(t *testing.T) {
	f := newPowerActuatorFixture(t)

	type testCase struct {
		name             string
		proactiveEnabled bool
		onlyPower        bool
		objectives       []string
		targets          map[string]float64
		result           map[string]map[string]float64
	}
	tests := []testCase{
		{name: "FitsExpectPerformance", objectives: []string{"default/p99latency"}, targets: map[string]float64{"default/p99latency": 50}, result: map[string]map[string]float64{"power.intel.com/performance": {"default/p99latency": 50}}},
		{name: "P99Dominant", objectives: []string{"default/p99latency", "default/p95latency"}, targets: map[string]float64{"default/p99latency": 50, "default/p95latency": 100}, result: map[string]map[string]float64{"power.intel.com/performance": {"default/p99latency": 50, "default/p95latency": 40}}},
		{name: "P95Dominant", objectives: []string{"default/p99latency", "default/p95latency"}, targets: map[string]float64{"default/p99latency": 200, "default/p95latency": 80}, result: map[string]map[string]float64{"power.intel.com/balance-performance": {"default/p99latency": 100, "default/p95latency": 80}}},
		{name: "FitsWithPowerOnly", objectives: []string{"default/p99latency", "default/my-power"}, targets: map[string]float64{"default/p99latency": 40, "default/my-power": 100}, result: map[string]map[string]float64{}},
		{name: "FitsWithBoth", objectives: []string{"default/p99latency", "default/my-power"}, targets: map[string]float64{"default/p99latency": 50, "default/my-power": 100}, result: map[string]map[string]float64{"power.intel.com/performance": {"default/p99latency": 50, "default/my-power": 40}}},
		{name: "Conflicts", objectives: []string{"default/p99latency", "default/my-power"}, targets: map[string]float64{"default/p99latency": 50, "default/my-power": 20}, result: map[string]map[string]float64{}},
		{name: "Unreachable", objectives: []string{"default/p99latency"}, targets: map[string]float64{"default/p99latency": 20}, result: map[string]map[string]float64{}},
		{name: "ConflictsProactive", proactiveEnabled: true, objectives: []string{"default/p99latency", "default/my-power"}, targets: map[string]float64{"default/p99latency": 50, "default/my-power": 20},
			result: map[string]map[string]float64{
				"None":                                {"default/p99latency": 200, "default/my-power": 10},
				"power.intel.com/balance-power":       {"default/p99latency": 150, "default/my-power": 20},
				"power.intel.com/balance-performance": {"default/p99latency": 100, "default/my-power": 30},
				"power.intel.com/performance":         {"default/p99latency": 50, "default/my-power": 40},
			},
		},
		{name: "UnreachableProactive", proactiveEnabled: true, objectives: []string{"default/p99latency"}, targets: map[string]float64{"default/p99latency": 20},
			result: map[string]map[string]float64{
				"None":                                {"default/p99latency": 200},
				"power.intel.com/balance-power":       {"default/p99latency": 150},
				"power.intel.com/balance-performance": {"default/p99latency": 100},
				"power.intel.com/performance":         {"default/p99latency": 50},
			},
		},
		{name: "Order0", proactiveEnabled: true, objectives: []string{"default/p99latency", "default/my-power"}, targets: map[string]float64{"default/p99latency": 75, "default/my-power": 10},
			result: map[string]map[string]float64{
				"None":                                {"default/p99latency": 200, "default/my-power": 10},
				"power.intel.com/balance-power":       {"default/p99latency": 150, "default/my-power": 20},
				"power.intel.com/balance-performance": {"default/p99latency": 100, "default/my-power": 30},
				"power.intel.com/performance":         {"default/p99latency": 50, "default/my-power": 40},
			},
		},
		{name: "Order1", proactiveEnabled: true, objectives: []string{"default/my-power", "default/p99latency"}, targets: map[string]float64{"default/my-power": 10, "default/p99latency": 75},
			result: map[string]map[string]float64{
				"None":                                {"default/p99latency": 200, "default/my-power": 10},
				"power.intel.com/balance-power":       {"default/p99latency": 150, "default/my-power": 20},
				"power.intel.com/balance-performance": {"default/p99latency": 100, "default/my-power": 30},
				"power.intel.com/performance":         {"default/p99latency": 50, "default/my-power": 40},
			},
		},
		{name: "EmptyResults", proactiveEnabled: true, objectives: []string{"default/my-super-power", "default/p99latency"}, targets: map[string]float64{"default/my-super-power": 10, "default/p99latency": 75}, result: map[string]map[string]float64{}},
		{name: "PowerOnly", onlyPower: true, objectives: []string{"default/my-power"}, targets: map[string]float64{"default/my-power": 30}, result: map[string]map[string]float64{"power.intel.com/balance-performance": {"default/my-power": 30}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actuator := f.newPowerTestActuator(tt.proactiveEnabled)
			defer f.cleanUp(actuator)
			res := actuator.findProfile("dummy", tt.objectives, tt.targets, 2000, tt.onlyPower)
			if !reflect.DeepEqual(res, tt.result) {
				t.Errorf("Expected %+v - got %+v.", tt.result, res)
			}
		})
	}
}

// TestPowerNextStateForSanity tests for sanity.
func TestPowerNextStateForSanity(t *testing.T) {
	f := newPowerActuatorFixture(t)
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	start := common.State{
		Intent: common.Intent{
			Key:        "default/my-app-intent",
			Priority:   1.0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{"default/p99latency": 180.0, "default/avail": 0.99},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {QoSClass: "Guaranteed"},
		},
		Resources:   map[string]int64{"0_cpu_requests": 1000},
		Annotations: map[string]string{"foo": "bar"},
		CurrentData: map[string]map[string]float64{
			"renewable_energy_ratio": {"node01": 1.0},
			actuator.Name():          {"newProfile": 1.0},
		},
	}
	goal := common.State{
		Intent: common.Intent{Objectives: map[string]float64{"default/p99latency": 60.0, "default/avail": 0.99}},
	}
	profiles := map[string]common.Profile{
		"default/p99latency": {ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/avail":      {ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
		"default/my-power":   {ProfileType: common.ProfileTypeFromText("power"), Minimize: true},
	}

	// in previous step we already did look at power...
	states, _, _ := actuator.NextState(&start, &goal, profiles)
	if len(states) != 0 {
		t.Errorf("Should have been empty: %v", states)
	}

	// we have no pods
	delete(start.CurrentData, actuator.Name())
	delete(start.CurrentPods, "pod_0")
	states, _, _ = actuator.NextState(&start, &goal, profiles)
	if len(states) != 0 {
		t.Errorf("Should have been empty: %v", states)
	}

	// we are in a BestEffort class.
	start.CurrentPods["pod_0"] = common.PodState{QoSClass: "BestEffort"}
	states, _, _ = actuator.NextState(&start, &goal, profiles)
	if len(states) != 0 {
		t.Errorf("Should not contain any entry: %v", states)
	}

	// this should work now...
	start.CurrentPods["pod_0"] = common.PodState{QoSClass: "Guaranteed"}
	states, utils, actions := actuator.NextState(&start, &goal, profiles)
	if len(states) != 1 {
		t.Errorf("Should contain one entry: %v", states)
	}
	if len(states) != len(utils) && len(utils) != len(actions) {
		t.Errorf("All lengths should be equal: %d, %d, %d", len(states), len(utils), len(actions))
	}
	_, ok := states[0].CurrentData[actuator.Name()]
	if !ok {
		t.Error("A flag should have been set, indicating a power tuning has been done...")
	}
	if actions[0].Properties.(map[string]string)["profile"] != "power.intel.com/performance" {
		t.Errorf("Action should indicate we want to switch to performance pool: %v", actions)
	}

	// already best power profile set...
	start.Resources["0_power.intel.com/performance_requests"] = 1000
	states, _, _ = actuator.NextState(&start, &goal, profiles)
	if len(states) != 0 {
		t.Errorf("Should return empty result...: %v", states)
	}

	// already have best power profile but resource allocations got updated...
	start.Resources["0_cpu_requests"] = 5000
	states, _, _ = actuator.NextState(&start, &goal, profiles)
	if len(states) != 1 {
		t.Errorf("Expected a single action...: %v", states)
	}

	// switch power profile
	start.Resources["0_power.intel.com/balance-power_requests"] = 1000
	delete(start.Resources, "0_power.intel.com/performance_requests")
	states, _, actions = actuator.NextState(&start, &goal, profiles)
	if len(states) != 1 {
		t.Errorf("Should return one result: %v", states)
	}
	if actions[0].Properties.(map[string]string)["profile"] != "power.intel.com/performance" {
		t.Errorf("Action should indicate we want to switch to performance pool: %v", actions)
	}

	// switch back to default/shared pool.
	goal.Intent.Objectives["default/p99latency"] = 250
	states, _, actions = actuator.NextState(&start, &goal, profiles)
	if len(states) != 1 {
		t.Errorf("Should return one result: %v", states)
	}
	if actions[0].Properties.(map[string]string)["profile"] != "None" {
		t.Errorf("Action should indicate we want to switch to shared pool: %v", actions)
	}

	// if none is set and best is none, do nothing!
	delete(start.Resources, "0_power.intel.com/balance-power_requests")
	states, _, actions = actuator.NextState(&start, &goal, profiles)
	if len(states) != 0 {
		t.Errorf("Should return an empty result: %v", actions)
	}

	// performance profile is set, but RER is below threshold, step down to balance-performance.
	start.Resources["0_power.intel.com/performance_requests"] = 1000
	start.CurrentData["renewable_energy_ratio"]["node01"] = 0.5
	goal.Intent.Objectives["default/p99latency"] = 60
	states, _, actions = actuator.NextState(&start, &goal, profiles)
	if len(states) != 1 {
		t.Errorf("Should return one result: %v", states)
	}
	if actions[0].Properties.(map[string]string)["profile"] != "power.intel.com/balance-performance" {
		t.Errorf("Action should indicate we want to switch to balance-performance pool: %v", actions)
	}

	// we are in balance-power now we need to step to None...
	delete(start.Resources, "0_power.intel.com/performance_requests")
	start.Resources["0_power.intel.com/balance-power_requests"] = 1000
	goal.Intent.Objectives["default/p99latency"] = 250
	states, _, actions = actuator.NextState(&start, &goal, profiles)
	if len(states) != 1 {
		t.Errorf("Should return one result: %v", states)
	}
	if actions[0].Properties.(map[string]string)["profile"] != "None" {
		t.Errorf("Action should indicate we want to switch to shared pool: %v", actions)
	}

	// let's enable proactive.
	actuator = f.newPowerTestActuator(true)
	start.CurrentData["renewable_energy_ratio"]["node01"] = 1.0
	delete(start.Resources, "0_power.intel.com/balance-power_requests")
	start.Intent.Objectives["default/my-power"] = 40
	goal.Intent.Objectives["default/p99latency"] = 75
	goal.Intent.Objectives["default/my-power"] = 10
	states, _, _ = actuator.NextState(&start, &goal, profiles)
	if len(states) != 4 {
		t.Errorf("Should return 4 result - one for each profile: %v", states)
	}
	for _, state := range states {
		if state.IsBetter(&goal, profiles) {
			t.Errorf("Should not be better: %v-%v", state, goal)
		}
	}

	// let's just work with a power profile.
	// TODO: work in progress.
}

// TestPowerPerformForSanity tests for sanity.
func TestPowerPerformForSanity(t *testing.T) {
	f := newPowerActuatorFixture(t)
	f.objects = []runtime.Object{f.createDeploymentObject("my-function")}
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	var plan []planner.Action
	state := common.State{
		Intent: common.Intent{
			TargetKey:  "default/my-function",
			TargetKind: "Deployment",
		},
	}

	// nothing in the plan for this actuator...
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 0 {
		t.Errorf("This is not expected - nothing should have happend...: %v", f.client.Actions())
	}

	// now this should work finally...
	f.client.ClearActions()
	plan = append(plan, planner.Action{Name: actuator.Name(), Properties: map[string]string{"profile": "power.intel.com/performance"}})
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 2 {
		t.Errorf("This is not expected - should see get & update...: %v", f.client.Actions())
	}
	updatedObject, _ := f.client.AppsV1().Deployments("default").Get(context.TODO(), "my-function", metaV1.GetOptions{})
	requests := updatedObject.Spec.Template.Spec.Containers[0].Resources.Requests
	_, ok := requests["power.intel.com/balance-power"]
	if ok {
		t.Errorf("This should no longer be here: %+v", requests)
	}
	_, ok = requests["power.intel.com/performance"]
	if !ok {
		t.Errorf("New power profile not set: %+v", requests)
	}

	// setting to None - should lead to removal of resources...
	f.client.ClearActions()
	plan = []planner.Action{{Name: actuator.Name(), Properties: map[string]string{"profile": "None"}}}
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 2 {
		t.Errorf("This is not expected - should see get & update...: %v", f.client.Actions())
	}
	updatedObject, _ = f.client.AppsV1().Deployments("default").Get(context.TODO(), "my-function", metaV1.GetOptions{})
	requests = updatedObject.Spec.Template.Spec.Containers[0].Resources.Requests
	for _, item := range actuator.config.PowerProfiles {
		_, ok = requests[v1.ResourceName(item)]
		if ok {
			t.Errorf("No power profile should be here; but was: %+v", requests)
		}
	}

	// make sure that all containers have set resource requirements...
	f.client.ClearActions()
	plan = []planner.Action{{Name: actuator.Name(), Properties: map[string]string{"profile": "power.intel.com/performance"}}}
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 2 {
		t.Errorf("This is not expected - should see get & update...: %v", f.client.Actions())
	}
	updatedObject, _ = f.client.AppsV1().Deployments("default").Get(context.TODO(), "my-function", metaV1.GetOptions{})
	for _, container := range updatedObject.Spec.Template.Spec.Containers {
		resRequest := container.Resources.Requests
		resLimit := container.Resources.Limits
		_, ok = resRequest[v1.ResourceName("power.intel.com/performance")]
		if !ok {
			t.Errorf("Should have set a performance profile in requests: %+v", resRequest)
		}
		_, ok = resLimit[v1.ResourceName("power.intel.com/performance")]
		if !ok {
			t.Errorf("Should have set a performance profile in limits: %+v", resLimit)
		}
	}

	// not a deployment...
	f.client.ClearActions()
	state.Intent.TargetKind = "ReplicaSet"
	actuator.Perform(&state, plan)
	if len(f.client.Actions()) != 0 {
		t.Errorf("This is not expected - should see nothing...: %v", f.client.Actions())
	}
}

// TestPowerEffectForSanity tests for sanity.
func TestPowerEffectForSanity(t *testing.T) {
	f := newPowerActuatorFixture(t)
	actuator := f.newPowerTestActuator(false)
	defer f.cleanUp(actuator)
	state := common.State{
		Intent: common.Intent{
			Objectives: map[string]float64{"default/p99latency": 100.0, "default/avail": 0.999, "default/my-pwr": 10.0},
		},
	}
	profiles := map[string]common.Profile{
		"default/p99latency": {ProfileType: common.ProfileTypeFromText("latency")},
		"default/avail":      {ProfileType: common.ProfileTypeFromText("availability")},
		"default/my-pwr":     {ProfileType: common.ProfileTypeFromText("power")},
	}
	actuator.Effect(&state, profiles)
}
