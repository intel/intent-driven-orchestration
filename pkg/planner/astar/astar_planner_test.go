package astar

import (
	"encoding/base64"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	plugins "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"

	"k8s.io/klog/v2"

	"crypto/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// waitGroup is a silly way to make sure we don't have race conditions in our test code.

// timeout for testing.
const timeout = 20

// aStarPlannerFixture represents a basic test fixture.
type aStarPlannerFixture struct {
	triggeredUpdates []string
	waitGroup        sync.WaitGroup
}

// newAStarPlannerFixture initializes a new testing fixture.
func newAStarPlannerFixture() *aStarPlannerFixture {
	f := &aStarPlannerFixture{}
	return f
}

// triggerUpdate channel function adds triggered updates to fixture.
func (f *aStarPlannerFixture) triggerUpdate() chan<- string {
	events := make(chan string)
	go func() {
		for {
			e := <-events
			f.waitGroup.Add(1)
			f.triggeredUpdates = append(f.triggeredUpdates, e)
			f.waitGroup.Done()
		}
	}()
	return events
}

// rmAction represents a dummy action to remove particular PODs.
type rmAction struct {
	actionTrigger chan<- string
}

// newRmAction initializes a remove pod action.
func newRmAction(ch chan<- string) rmAction {
	return rmAction{actionTrigger: ch}
}

func (rm rmAction) Name() string {
	return "rm_pod"
}

func (rm rmAction) Group() string {
	return "scaling"
}

func (rm rmAction) NextState(state *common.State, _ *common.State, _ map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	var followUpStates []common.State
	var utilities []float64
	var actions []planner.Action

	for pod := range state.CurrentPods {
		if strings.Contains(pod, "dummy_") {
			// no need to remove dummy pods...
			continue
		}
		newState := state.DeepCopy()
		delete(newState.CurrentPods, pod)

		numPods := len(newState.CurrentPods)
		if numPods < 1 {
			break
		}

		// ... update predicted objectives...
		newState.Intent.Objectives["p99latency"] = 120
		if numPods == 2 {
			newState.Intent.Objectives["p99latency"] = 90
		} else if numPods == 3 {
			newState.Intent.Objectives["p99latency"] = 40
		} else if numPods == 4 {
			newState.Intent.Objectives["p99latency"] = 30
		}

		followUpStates = append(followUpStates, newState)
		utilities = append(utilities, state.CurrentPods[pod].Availability)
		actions = append(actions, planner.Action{
			Name:       rm.Name(),
			Properties: map[string]string{"pod": pod}},
		)
	}

	return followUpStates, utilities, actions
}

func (rm rmAction) Perform(_ *common.State, plan []planner.Action) {
	for _, action := range plan {
		if action.Name == rm.Name() {
			rm.actionTrigger <- rm.Name()
			return
		}
	}
}

func (rm rmAction) Effect(state *common.State, _ map[string]common.Profile) {
	for objective := range state.Intent.Objectives {
		rm.actionTrigger <- objective + "_" + rm.Name()
	}
}

// scaleAction represents a dummy action for scaling workloads.
type scaleAction struct {
	actionTrigger chan<- string
}

// newScaleAction initializes a remove pod action.
func newScaleAction(ch chan<- string) scaleAction {
	return scaleAction{actionTrigger: ch}
}

func (scale scaleAction) Name() string {
	return "set_replicas"
}

func (scale scaleAction) Group() string {
	return "scaling"
}

func (scale scaleAction) NextState(state *common.State, goal *common.State, _ map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	var followUpStates []common.State
	var utilities []float64
	var actions []planner.Action

	// "predict" the required number of replicas
	numPods := 1
	if goal.Intent.Objectives["p99latency"] <= 50 {
		numPods = 3
	} else if goal.Intent.Objectives["p99latency"] <= 100 {
		numPods = 2
	}

	// if we need to change sth...
	if len(state.CurrentPods) < numPods {
		newState := state.DeepCopy()
		// ... add dummy pods ...
		j := 0
		for k := range state.CurrentPods {
			if strings.Contains(k, "dummy_") {
				j++
			}
		}
		for i := len(state.CurrentPods); i < numPods; i++ {
			newState.CurrentPods["dummy_"+strconv.Itoa(j)] = common.PodState{
				Resources:    nil,
				Annotations:  nil,
				Availability: 1.0,
				NodeName:     "",
				State:        "Running",
			}
			j++
		}

		// ... update predicted objectives...
		newState.Intent.Objectives["p99latency"] = 120
		if numPods == 2 {
			newState.Intent.Objectives["p99latency"] = 90
		} else if numPods == 3 {
			newState.Intent.Objectives["p99latency"] = 40
		} else if numPods == 4 {
			newState.Intent.Objectives["p99latency"] = 30
		}

		// ... and add to result sets.
		followUpStates = append(followUpStates, newState)
		utilities = append(utilities, 1.0)
		actions = append(actions, planner.Action{
			Name:       scale.Name(),
			Properties: map[string]string{"replicas": strconv.Itoa(numPods)}},
		)
	}
	return followUpStates, utilities, actions
}

func (scale scaleAction) Perform(_ *common.State, plan []planner.Action) {
	for _, action := range plan {
		if action.Name == scale.Name() {
			scale.actionTrigger <- scale.Name()
			return
		}
	}
}

func (scale scaleAction) Effect(state *common.State, _ map[string]common.Profile) {
	for objective := range state.Intent.Objectives {
		scale.actionTrigger <- objective + "_" + scale.Name()
	}
}

// faultyAction represents a misbehaving actuator.
type faultyAction struct {
	counter chan<- string
}

// newFaultyAction initializes a remove pod action.
func newFaultyAction(ch chan<- string) faultyAction {
	return faultyAction{counter: ch}
}

func (faulty faultyAction) Name() string {
	return "dawdle"
}

func (faulty faultyAction) Group() string {
	return "faulty"
}

func (faulty faultyAction) NextState(state *common.State, _ *common.State, _ map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	var followUpStates []common.State
	var utilities []float64
	var actions []planner.Action

	i := 0
	for i < 10 {
		newState := state.DeepCopy()
		buff := make([]byte, 8)
		_, err := rand.Read(buff)
		if err != nil {
			klog.Error(err)
		}
		str := base64.StdEncoding.EncodeToString(buff)
		newState.CurrentPods[str] = common.PodState{}

		followUpStates = append(followUpStates, newState)
		utilities = append(utilities, 0.0)
		actions = append(actions, planner.Action{Name: faultyAction{}.Name()})

		i++
	}

	faulty.counter <- faulty.Name()

	return followUpStates, utilities, actions
}

func (faulty faultyAction) Perform(_ *common.State, _ []planner.Action) {
	klog.Fatal("implement me")
}

func (faulty faultyAction) Effect(_ *common.State, _ map[string]common.Profile) {
	klog.Fatalf("implement me")
}

// newTestPlanner
func (f *aStarPlannerFixture) newTestPlanner(enableOpportunistic bool) *APlanner {
	channel := f.triggerUpdate()
	actuatorList := []actuators.Actuator{newScaleAction(channel), newRmAction(channel)}
	cfg := common.Config{Generic: common.GenericConfig{MongoEndpoint: controller.MongoURIForTesting}}
	cfg.Planner.AStar.MaxCandidates = 10
	cfg.Planner.AStar.MaxStates = 1000
	if enableOpportunistic == true {
		cfg.Planner.AStar.OpportunisticCandidates = 2
	}
	aPlanner := NewAPlanner(actuatorList, cfg)
	return aPlanner
}

func createPlugin(name string, port int, actuator actuators.Actuator, serverPort int) (*plugins.ActuatorPluginStub, error) {
	stub := plugins.NewActuatorPluginStub(name, "localhost", port, "localhost", serverPort)
	stub.SetNextStateFunc(actuator.NextState)
	stub.SetPerformFunc(actuator.Perform)
	stub.SetEffectFunc(actuator.Effect)
	err := stub.Start()
	if err != nil {
		return nil, err
	}
	err = stub.Register()
	if err != nil {
		err = stub.Stop()
		return nil, err
	}
	return stub, nil
}

// newTestPlannerGrpc
func (f *aStarPlannerFixture) newTestPlannerGrpc(enableOpportunistic bool) *APlanner {
	cfg := common.Config{Generic: common.GenericConfig{MongoEndpoint: controller.MongoURIForTesting}}
	cfg.Planner.AStar.MaxCandidates = 10
	cfg.Planner.AStar.MaxStates = 1000
	cfg.Planner.AStar.PluginManagerEndpoint = "localhost"
	cfg.Planner.AStar.PluginManagerPort = 33333
	if enableOpportunistic == true {
		cfg.Planner.AStar.OpportunisticCandidates = 2
	}
	aPlanner := NewAPlanner([]actuators.Actuator{}, cfg)

	return aPlanner
}

func newTestActuatorsGrpc(f *aStarPlannerFixture) []*plugins.ActuatorPluginStub {
	channel := f.triggerUpdate()

	scale, err := createPlugin("scale_out", 3339, newScaleAction(channel), 33333)
	if err != nil {
		klog.Errorf("Cannot create scale plugin over grpc")
		return []*plugins.ActuatorPluginStub{}
	}
	rmpod, err := createPlugin("rm_pod", 3338, newRmAction(channel), 33333)
	if err != nil {
		errs := scale.Stop()
		if errs != nil {
			klog.Errorf("Cannot stop scale")
		}
		klog.Errorf("Cannot create rm plugin over grpc")
		return []*plugins.ActuatorPluginStub{}
	}
	return []*plugins.ActuatorPluginStub{scale, rmpod}
}

type testCaseData struct {
	name       string
	fixture    *aStarPlannerFixture
	plannerCrt func(*aStarPlannerFixture) *APlanner
	planner    *APlanner
	stubsCrt   func(*aStarPlannerFixture) []*plugins.ActuatorPluginStub
	stubs      []*plugins.ActuatorPluginStub
}

func (tc testCaseData) stop() {
	for _, s := range tc.stubs {
		err := s.Stop()
		if err != nil {
			klog.Errorf("Cannot stop stub")
		}
	}
}

func getPlannerTestCases(enableOpportunistic bool) []testCaseData {
	f1 := newAStarPlannerFixture()
	f2 := newAStarPlannerFixture()

	return []testCaseData{
		{
			name:       "local actuators",
			fixture:    f1,
			plannerCrt: func(f *aStarPlannerFixture) *APlanner { return f1.newTestPlanner(enableOpportunistic) },
			stubsCrt:   func(f *aStarPlannerFixture) []*plugins.ActuatorPluginStub { return []*plugins.ActuatorPluginStub{} },
		},
		{
			name:       "grpc actuators",
			fixture:    f2,
			plannerCrt: func(f *aStarPlannerFixture) *APlanner { return f2.newTestPlannerGrpc(enableOpportunistic) },
			stubsCrt:   func(f *aStarPlannerFixture) []*plugins.ActuatorPluginStub { return newTestActuatorsGrpc(f2) },
		},
	}
}

// Tests for success.

// TestGetNodeForStateForSuccess tests for sanity.
func TestGetNodeForStateForSuccess(t *testing.T) {
	sg := newStateGraph()
	s0 := common.State{}
	getNodeForState(*sg, s0)
}

// TestCreatePlanForSuccess tests for success.
func TestCreatePlanForSuccess(t *testing.T) {
	f := newAStarPlannerFixture()
	start := common.State{
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
	}
	goal := common.State{
		Intent: common.Intent{
			Key:        "goal",
			Priority:   0.0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99latency": 100,
			}},
		CurrentPods: nil,
		CurrentData: nil,
	}

	profiles := map[string]common.Profile{"p99latency": {ProfileType: common.ProfileTypeFromText("latency")}}
	aPlanner := f.newTestPlanner(false)
	defer aPlanner.Stop()
	aPlanner.CreatePlan(start, goal, profiles)
}

// TestExecutePlanForSuccess tests for success.
func TestExecutePlanForSuccess(t *testing.T) {
	f := newAStarPlannerFixture()
	state := common.State{Intent: common.Intent{
		Key:        "foo",
		Priority:   0,
		TargetKey:  "",
		TargetKind: "",
		Objectives: map[string]float64{"p99": 100},
	}}
	aPlanner := f.newTestPlanner(false)
	defer aPlanner.Stop()
	var plan []planner.Action
	aPlanner.ExecutePlan(state, plan)
}

// TestTriggerEffectForSuccess tests for success.
func TestTriggerEffectForSuccess(t *testing.T) {
	f := newAStarPlannerFixture()
	state := common.State{Intent: common.Intent{
		Key:        "foo",
		Priority:   0,
		TargetKey:  "",
		TargetKind: "",
		Objectives: map[string]float64{"p99": 100},
	}}
	profiles := map[string]common.Profile{"p99": {ProfileType: common.ProfileTypeFromText("latency")}}
	aPlanner := f.newTestPlanner(false)
	defer aPlanner.Stop()
	aPlanner.TriggerEffect(state, profiles)
}

// Tests for failure.

// n/a.

// Tests for sanity.

// TestGetNodeForStateForSanity tests for sanity.
func TestGetNodeForStateForSanity(t *testing.T) {
	sg := newStateGraph()
	state0 := common.State{Intent: common.Intent{
		Key:        "foo",
		Priority:   0,
		TargetKey:  "",
		TargetKind: "",
		Objectives: map[string]float64{"p99": 100},
	}}
	state1 := common.State{Intent: common.Intent{
		Key:        "foo",
		Priority:   0,
		TargetKey:  "",
		TargetKind: "",
		Objectives: map[string]float64{"p99": 100},
	}}
	state2 := common.State{Intent: common.Intent{
		Key:        "foo",
		Priority:   0,
		TargetKey:  "",
		TargetKind: "",
		Objectives: map[string]float64{"p99": 90},
	}}
	node0 := Node{&state0}
	node1 := Node{&state1}
	sg.addNode(node0)
	sg.addNode(node1)

	if reflect.DeepEqual(state0, state1) != true || reflect.DeepEqual(state0, state2) != false || reflect.DeepEqual(state1, state2) != false {
		t.Errorf("Something is off here - DeepEqual seems not to have worked properly!")
	}

	res, found := getNodeForState(*sg, state0)
	if found == false || res != node1 { // node1 b/c we do traverse list back to front!
		klog.Info("Whoops0")
	}
	res, found = getNodeForState(*sg, state1)
	if found == false || res != node1 {
		klog.Info("Whoops1")
	}
	_, found = getNodeForState(*sg, state2)
	if found == true {
		klog.Info("Whoops2")
	}
}

// TestCreatePlanForSuccess tests for sanity.
func TestCreatePlanForSanity(t *testing.T) {
	testCases := getPlannerTestCases(false)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			start := common.State{
				Intent: common.Intent{
					Key:        "test-my-objective",
					Priority:   0.0,
					TargetKey:  "my-deployment",
					TargetKind: "Deployment",
					Objectives: map[string]float64{
						"p99latency": 150,
					},
				},
				CurrentPods: map[string]common.PodState{"pod_0": {Availability: 0.7}, "pod_1": {Availability: 1.0}},
				CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20.0}},
			}
			goal := common.State{
				Intent: common.Intent{
					Key:        "goal",
					Priority:   0.0,
					TargetKey:  "",
					TargetKind: "",
					Objectives: map[string]float64{
						"p99latency": 50,
					}},
				CurrentPods: nil,
				CurrentData: nil,
			}
			profiles := map[string]common.Profile{"p99latency": {ProfileType: common.ProfileTypeFromText("latency")}}
			testCase.planner = testCase.plannerCrt(testCase.fixture)
			testCase.stubs = testCase.stubsCrt(testCase.fixture)
			res := testCase.planner.CreatePlan(start, goal, profiles)

			expectedActions := []string{"set_replicas"}
			for i, action := range expectedActions {
				if action != res[i].Name {
					t.Errorf("Expected '%s' got: '%s'.", action, res[i].Name)
				}
			}

			// no path to goal state possible.
			start1 := start.DeepCopy()
			delete(start1.Intent.Objectives, "p99latency")
			start1.Intent.Objectives["p50"] = 100
			res = testCase.planner.CreatePlan(start1, goal, profiles)
			if len(res) != 0 {
				t.Errorf("Expected length of plan to be 0 - got %v", res)
			}

			// start is better than goal.
			start2 := start.DeepCopy()
			start2.Intent.Objectives["p99latency"] = 45
			start2.CurrentPods["foo"] = common.PodState{Availability: 1.0}
			start2.CurrentPods["bar"] = common.PodState{Availability: 1.0}
			res = testCase.planner.CreatePlan(start2, goal, profiles)
			if len(res) != 1 {
				// rm & done.
				t.Errorf("Expected length of plan to be 1 - got %v", res)
			}
		})
		testCase.stop()
		testCase.planner.Stop()
	}
}

// TestOpportunisticPlannerForSanity tests for sanity
func TestOpportunisticPlannerForSanity(t *testing.T) {
	testCases := getPlannerTestCases(true)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			start := common.State{
				Intent: common.Intent{
					Key:        "test-my-objective",
					Priority:   0.0,
					TargetKey:  "my-deployment",
					TargetKind: "Deployment",
					Objectives: map[string]float64{
						"p99latency": 150,
					},
				},
				CurrentPods: map[string]common.PodState{"pod_0": {Availability: 0.7}, "pod_1": {Availability: 1.0}},
				CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20.0}},
			}
			goal := common.State{
				Intent: common.Intent{
					Key:        "goal",
					Priority:   0.0,
					TargetKey:  "",
					TargetKind: "",
					Objectives: map[string]float64{
						"p99latency": 5,
					}},
				CurrentPods: nil,
				CurrentData: nil,
			}
			profiles := map[string]common.Profile{"p99latency": {ProfileType: common.ProfileTypeFromText("latency")}}
			testCase.planner = testCase.plannerCrt(testCase.fixture)
			testCase.stubs = testCase.stubsCrt(testCase.fixture)
			res := testCase.planner.CreatePlan(start, goal, profiles)
			expectedActions := []string{"set_replicas"}
			if len(res) != 1 {
				t.Errorf("Expected at least one action - got: %d.", len(res))
			}
			for i, item := range res {
				if item.Name != expectedActions[i] {
					t.Errorf("Not the action we expected wanted %s - got %s", expectedActions[i], item.Name)
				}
			}
		})
		testCase.stop()
		testCase.planner.Stop()
	}
}

// TestFaultyActuatorForSanity tests for sanity
func TestFaultyActuatorForSanity(t *testing.T) {
	f := newAStarPlannerFixture()
	channel := f.triggerUpdate()
	faulty := newFaultyAction(channel)
	actuatorList := []actuators.Actuator{faulty}
	cfg := common.Config{Generic: common.GenericConfig{MongoEndpoint: controller.MongoURIForTesting}}
	cfg.Planner.AStar.MaxCandidates = 2
	cfg.Planner.AStar.MaxStates = 10
	aPlanner := NewAPlanner(actuatorList, cfg)
	defer aPlanner.Stop()

	start := common.State{}
	goal := common.State{}

	profiles := map[string]common.Profile{"p72.5": {ProfileType: common.ProfileTypeFromText("latency")}}
	res := aPlanner.CreatePlan(start, goal, profiles)
	if len(res) != 0 {
		t.Errorf("Plan should be empty.")
	}
	time.Sleep(timeout * time.Millisecond)
	f.waitGroup.Wait()
	if len(f.triggeredUpdates) != 4 {
		t.Errorf("Expected to have NextState to be called 4 times - max graph size is 10 - includeing start "+
			"and goal, that means Nextstate() should be called 4 times as we only take 2 candidates from each call. "+
			"Was: %v", len(f.triggeredUpdates))
	}
}

// TestFaultyActuatorForSanity tests for sanity
func TestFaultyActuatorOverGrpcForSanity(t *testing.T) {
	f := newAStarPlannerFixture()
	channel := f.triggerUpdate()
	faulty := newFaultyAction(channel)
	var actuatorList []actuators.Actuator
	cfg := common.Config{Generic: common.GenericConfig{MongoEndpoint: controller.MongoURIForTesting}}
	cfg.Planner.AStar.MaxCandidates = 2
	cfg.Planner.AStar.MaxStates = 10
	cfg.Planner.AStar.PluginManagerEndpoint = "localhost"
	cfg.Planner.AStar.PluginManagerPort = 33337
	aPlanner := NewAPlanner(actuatorList, cfg)

	faultyStub, err := createPlugin("faulty", 3338, faulty, 33337)
	if err != nil {
		klog.Errorf("Cannot create faulty plugin over grpc")
	}
	defer aPlanner.Stop()

	start := common.State{}
	goal := common.State{}

	profiles := map[string]common.Profile{"p72.5": {ProfileType: common.ProfileTypeFromText("latency")}}
	res := aPlanner.CreatePlan(start, goal, profiles)
	if len(res) != 0 {
		t.Errorf("Plan should be empty.")
	}
	time.Sleep(timeout * time.Millisecond)
	f.waitGroup.Wait()
	err = faultyStub.Stop()
	if err != nil {
		klog.Errorf("Cannot stop faulty stub")
	}
	if len(f.triggeredUpdates) != 4 {
		t.Errorf("Expected to have NextState to be called 4 times - max graph size is 10 - includeing start "+
			"and goal, that means Nextstate() should be called 4 times as we only take 2 candidates from each call. "+
			"Was: %v", len(f.triggeredUpdates))
	}

}

// BenchmarkCreatePlan benchmarks the planner.
func BenchmarkCreatePlan(b *testing.B) {
	f := newAStarPlannerFixture()
	start := common.State{
		Intent: common.Intent{
			Key:        "test-my-objective",
			Priority:   0.0,
			TargetKey:  "my-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"p99latency": 150,
			},
		},
		CurrentPods: map[string]common.PodState{"pod_0": {Availability: 0.7}, "pod_1": {Availability: 1.0}},
		CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20.0}},
	}
	goal := common.State{
		Intent: common.Intent{
			Key:        "goal",
			Priority:   0.0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99latency": 50,
			}},
		CurrentPods: nil,
		CurrentData: nil,
	}
	profiles := map[string]common.Profile{"p99latency": {ProfileType: common.ProfileTypeFromText("latency")}}

	plnr := f.newTestPlanner(false)
	defer plnr.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plnr.CreatePlan(start, goal, profiles)
	}
}

// TestExecutePlanForSanity tests for sanity.
func TestExecutePlanForSanity(t *testing.T) {
	testCases := getPlannerTestCases(false)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := common.State{
				Intent: common.Intent{
					Key:        "test-my-objective",
					Priority:   0.0,
					TargetKey:  "my-deployment",
					TargetKind: "Deployment",
					Objectives: map[string]float64{
						"p99latency": 120,
					},
				},
				CurrentPods: map[string]common.PodState{"pod_0": {Availability: 0.7}, "pod_1": {Availability: 1.0}},
				CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20.0}},
			}
			plan := []planner.Action{{Name: "rm_pod", Properties: nil}}
			if len(testCase.fixture.triggeredUpdates) != 0 {
				t.Errorf("No action should be called prior to ExecutePlan call")
			}
			testCase.planner = testCase.plannerCrt(testCase.fixture)
			testCase.stubs = testCase.stubsCrt(testCase.fixture)
			testCase.planner.ExecutePlan(state, plan)
			time.Sleep(timeout * time.Millisecond)
			testCase.fixture.waitGroup.Wait()
			if testCase.fixture.triggeredUpdates[0] != "rm_pod" {
				t.Errorf("Expected rm_pod action to have been called!")
			}
		})
		testCase.stop()
		testCase.planner.Stop()
	}
}

// TestTriggerEffectForSanity tests for sanity.
func TestTriggerEffectForSanity(t *testing.T) {
	testCases := getPlannerTestCases(false)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := common.State{
				Intent: common.Intent{
					Key:        "test-my-objective",
					Priority:   0.0,
					TargetKey:  "my-deployment",
					TargetKind: "Deployment",
					Objectives: map[string]float64{
						"p95latency": 120,
						"p99latency": 120,
					},
				},
			}
			testCase.planner = testCase.plannerCrt(testCase.fixture)
			testCase.stubs = testCase.stubsCrt(testCase.fixture)
			testCase.planner.TriggerEffect(state, nil)
			time.Sleep(timeout * time.Millisecond)
			testCase.fixture.waitGroup.Wait()
			expected := map[string]bool{
				"p99latency_set_replicas": true,
				"p99latency_rm_pod":       true,
				"p95latency_set_replicas": true,
				"p95latency_rm_pod":       true,
			}
			for _, item := range testCase.fixture.triggeredUpdates {
				if _, ok := expected[item]; !ok {
					t.Errorf("Got unexpected update: %s.", item)
				}
			}
		})
		testCase.stop()
		testCase.planner.Stop()
	}
}
