package controller

import (
	"reflect"
	"testing"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

// dummyTracer for testing.
type dummyTracer struct{}

func (d dummyTracer) GetEffect(string, string, string, int, func() interface{}) (interface{}, error) {
	return nil, nil
}

func (d dummyTracer) TraceEvent(_ common.State, _ common.State, _ []planner.Action) {
	klog.Info("TraceEvent called.")
}

// dummyPlanner for testing.
type dummyPlanner struct{}

func (d dummyPlanner) CreatePlan(_ common.State, _ common.State, _ map[string]common.Profile) []planner.Action {
	return []planner.Action{{Name: "test"}, {Name: "done"}}
}

func (d dummyPlanner) ExecutePlan(_ common.State, _ []planner.Action) {
	klog.Info("Execute called.")
}

func (d dummyPlanner) TriggerEffect(_ common.State, _ map[string]common.Profile) {
	klog.Info("Trigger called.")
}

// newTestController returns a controller ready for testing.
func newTestController() *IntentController {
	controllerConfig := common.ControllerConfig{
		TaskChannelLength: 100,
		ControllerTimeout: 1,
		PlanCacheTimeout:  100,
		PlanCacheTTL:      10,
	}
	genericConfig := common.GenericConfig{
		MongoEndpoint: MongoURIForTesting,
	}
	cfg := common.Config{Generic: genericConfig, Controller: controllerConfig}
	client := fake.NewSimpleClientset(nil...)
	informer := informers.NewSharedInformerFactory(client, func() time.Duration { return 0 }())
	dummyPlanner := dummyPlanner{}
	controller := NewController(cfg, nil, informer.Core().V1().Pods())
	controller.SetPlanner(dummyPlanner)
	controller.tracer = dummyTracer{}
	return controller
}

// Tests for success.

// TestUpdateProfileForSuccess tests for success.
func TestUpdateProfileForSuccess(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)
	c.UpdateProfile() <- common.Profile{Key: "test", ProfileType: common.Latency, Query: "foo", External: true}
}

// TestUpdatePodErrorForSuccess tests for success.
func TestUpdatePodErrorForSuccess(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)
	c.UpdatePodError() <- common.PodError{Key: "test", Start: time.Now(), End: time.Now()}
}

// TestUpdateIntentForSuccess tests for success.
func TestUpdateIntentForSuccess(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)

	c.UpdateIntent() <- common.Intent{Key: "test", Priority: 1.0, TargetKey: "frontend", TargetKind: "Deployment"}
}

// TestRunForSuccess tests for success.
func TestRunControllerForSuccess(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)
}

// TestProcessIntentsForSuccess tests for success.
func TestProcessIntentsForSuccess(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.processIntents()
}

// Tests for failure.

// n/a

// Tests for sanity.

// TestUpdateProfileForSuccess tests for sanity.
func TestUpdateProfileForSanity(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)

	c.UpdateProfile() <- common.Profile{Key: "test", ProfileType: common.Latency, Query: "foo", External: true}
	c.profilesLock.Lock()
	if len(c.profiles) != 1 {
		t.Error("Profile not added to profiles map.")
	}
	c.profilesLock.Unlock()

	c.UpdateProfile() <- common.Profile{Key: "test", ProfileType: common.Obsolete, Query: "foo", External: true}
	c.profilesLock.Lock()
	if len(c.profiles) != 0 {
		t.Error("Profile should have been removed.")
	}
	c.profilesLock.Unlock()
}

// TestUpdatePodErrorForSanity tests for sanity.
func TestUpdatePodErrorForSanity(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)

	c.UpdatePodError() <- common.PodError{Key: "test", Start: time.Now(), End: time.Now()}
	c.podErrorLock.Lock()
	if len(c.podErrors["test"]) == 0 {
		t.Error("These should be a POD error in the map.")
	}
	c.podErrorLock.Unlock()

	c.UpdatePodError() <- common.PodError{Key: "test"}
	c.podErrorLock.Lock()
	if len(c.podErrors) != 0 {
		t.Error("Should removed the POD from the map.")
	}
	c.podErrorLock.Unlock()
}

// TestUpdateIntentForSanity tests for sanity.
func TestUpdateIntentForSanity(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)

	c.UpdateIntent() <- common.Intent{Key: "test", Priority: 1.0, TargetKey: "frontend", TargetKind: "Deployment"}
	c.intentsLock.Lock()
	if len(c.intents) != 1 {
		t.Error("Intent has not been added to intents map.")
	}
	c.intentsLock.Unlock()

	c.UpdateIntent() <- common.Intent{Key: "test", Priority: -1.0}
	c.intentsLock.Lock()
	if len(c.intents) != 0 {
		t.Error("Intent should have been removed.")
	}
	c.intentsLock.Unlock()
}

// TestRunControllerForSanity tests for sanity.
func TestRunControllerForSanity(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)

	time.Sleep(time.Duration(c.cfg.Controller.ControllerTimeout)*time.Second + time.Second)
	// TODO: add asserts!
}

// TestProcessIntentsForSanity tests for sanity.
func TestProcessIntentsForSanity(t *testing.T) {
	stopChannel := make(chan struct{})
	defer close(stopChannel)
	c := newTestController()
	c.Run(1, stopChannel)

	c.UpdateIntent() <- common.Intent{Key: "foo", TargetKey: "foo", TargetKind: "bar"}
	time.Sleep(time.Duration(c.cfg.Controller.ControllerTimeout)*time.Second + time.Second)
	c.processIntents()
	time.Sleep(time.Duration(c.cfg.Controller.ControllerTimeout)*time.Second + time.Second)
}

// TestNewControllerForFailure tests for sanity.
func TestNewControllerForFailure(t *testing.T) {
	type args struct {
		cfg       common.Config
		clientSet *kubernetes.Clientset
		informer  v1.PodInformer
	}
	genericConfig := common.GenericConfig{
		MongoEndpoint: MongoURIForTesting,
	}
	tests := []struct {
		name string
		args args
		want interface{}
	}{
		{name: "tc1",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: 100,
					ControllerTimeout: 1,
					PlanCacheTimeout:  100,
					PlanCacheTTL:      10,
				}},
			},
			want: nil},
		{name: "tc2",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: -100,
					ControllerTimeout: 1,
					PlanCacheTimeout:  100,
					PlanCacheTTL:      10,
				}},
			},
			want: nil},
		{name: "tc3",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: -100,
					ControllerTimeout: -1,
					PlanCacheTimeout:  100,
					PlanCacheTTL:      10,
				}},
			},
			want: nil},
		{name: "tc4",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: -100,
					ControllerTimeout: -1,
					PlanCacheTimeout:  -100,
					PlanCacheTTL:      10,
				}},
			},
			want: nil},
		{name: "tc5",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: -100,
					ControllerTimeout: -1,
					PlanCacheTimeout:  -100,
					PlanCacheTTL:      -10,
				}},
			},
			want: nil},
		{name: "tc6",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: 0,
					ControllerTimeout: -1,
					PlanCacheTimeout:  -100,
					PlanCacheTTL:      -10,
				}},
			},
			want: nil},
		{name: "tc7",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: 34,
					ControllerTimeout: -19,
					PlanCacheTimeout:  -32,
					PlanCacheTTL:      10,
				}},
			},
			want: nil},
		{name: "tc8",
			args: args{cfg: common.Config{
				Generic: genericConfig, Controller: common.ControllerConfig{
					TaskChannelLength: 0x80000000,
					ControllerTimeout: 0xfffffffe,
					PlanCacheTimeout:  0x7fffffff,
					PlanCacheTTL:      0x7ffffffe,
				}},
			},
			want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := NewController(tt.args.cfg, tt.args.clientSet, tt.args.informer)
			if controller != nil {
				if got := controller.planner; !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Planner in NewController() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
