package tests

import (
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/scaling"
	"github.com/intel/intent-driven-orchestration/pkg/planner/astar"

	"k8s.io/klog/v2"
)

// dummyTracer allows us to control what information we give to the actuator.
type dummyTracer struct{}

func (d dummyTracer) TraceEvent(_ common.State, _ common.State, _ []planner.Action) {
	klog.Fatal("implement me")
}

func (d dummyTracer) GetEffect(_ string, _ string, _ string, _ int, constructor func() interface{}) (interface{}, error) {
	tmp := constructor().(*scaling.ScaleOutEffect)
	// these numbers are from a test of the recommendation service of the GCP online boutique microservice demo app.
	tmp.ReplicaRange = []int{1, 12}
	// Note that back in the days we did measure latency in s not ms.
	tmp.Popt = []float64{0.02, 0.0000005, 0.008}
	tmp.ThroughputRange = []float64{0.3, 6064.0}
	return tmp, nil
}

func setupTestCase() (common.State, common.State, map[string]common.Profile) {
	start := common.State{
		Intent: common.Intent{
			Objectives: map[string]float64{
				"p99":          0.04,
				"rps":          4800,
				"availability": 0.998,
			},
		},
		CurrentPods: map[string]common.PodState{
			"pod_0": {State: "Running", Availability: 0.9},
			"pod_1": {State: "Running", Availability: 0.98},
		},
	}
	goal := common.State{
		Intent: common.Intent{
			Objectives: map[string]float64{
				"p99":          0.03,
				"rps":          0,
				"availability": 0.999,
			},
			Priority: 1.0,
		},
	}
	profiles := map[string]common.Profile{
		"p99":          {ProfileType: common.ProfileTypeFromText("latency")},
		"rps":          {ProfileType: common.ProfileTypeFromText("throughput")},
		"availability": {ProfileType: common.ProfileTypeFromText("availability")},
	}
	return start, goal, profiles
}

func executeBenchmark(b *testing.B, planner *astar.APlanner) {
	start, goal, profiles := setupTestCase()

	// quick test if the planner actually does sth...
	res := planner.CreatePlan(start, goal, profiles)
	if len(res) != 1 || res[0].Name != "scaleOut" || res[0].Properties.(map[string]int32)["factor"] != 2 {
		klog.Errorf("benchmarks will fail - planner did not run correctly; result was: %v.", res)
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		planner.CreatePlan(start, goal, profiles)
	}
	b.StopTimer()
}

// BenchmarkAStarGrpcCreatePlan benchmarks the planner including grpc actuator.
func BenchmarkAStarGrpcCreatePlan(b *testing.B) {
	cfg := common.Config{}
	cfg.Planner.AStar.MaxCandidates = 10
	cfg.Planner.AStar.MaxStates = 5000
	cfg.Planner.AStar.PluginManagerEndpoint = "localhost"
	cfg.Planner.AStar.PluginManagerPort = 33339

	tracer := dummyTracer{}

	var actuatorList []actuators.Actuator
	myPlanner := astar.NewAPlanner(actuatorList, cfg)
	defer myPlanner.Stop()
	pS := startScaleOutPlugin(tracer, 3335, 33339)
	pR := startRemovePodPlugin(tracer, 3337, 33339)

	b.Run("", func(b *testing.B) {
		executeBenchmark(b, myPlanner)
	})

	err := pS.Stop()
	if err != nil {
		klog.ErrorS(err, "error stopping scale_out")
	}
	err = pR.Stop()
	if err != nil {
		klog.ErrorS(err, "error stopping rm_pod")
	}
}

// BenchmarkAStarCreatePlan benchmarks the planner including actuators.
func BenchmarkAStarCreatePlan(b *testing.B) {
	cfg := common.Config{}
	cfg.Planner.AStar.MaxCandidates = 10
	cfg.Planner.AStar.MaxStates = 5000

	rmPodCfg := scaling.RmPodConfig{}
	rmPodCfg.LookBack = 10
	rmPodCfg.MinPods = 1

	scaleCfg := scaling.ScaleOutConfig{}
	scaleCfg.MaxPods = 128

	tracer := dummyTracer{}

	var actuatorList []actuators.Actuator
	actuatorList = append(actuatorList, scaling.NewScaleOutActuator(nil, tracer, scaleCfg))
	actuatorList = append(actuatorList, scaling.NewRmPodActuator(nil, tracer, rmPodCfg))

	myPlanner := astar.NewAPlanner(actuatorList, cfg)
	defer myPlanner.Stop()
	b.Run("", func(b *testing.B) {
		executeBenchmark(b, myPlanner)
	})
}

// TestAStarGrpcCreatePlan test the planner including grpc actuator.
func TestAStarGrpcCreatePlan(t *testing.T) {
	cfg := common.Config{}
	cfg.Planner.AStar.MaxCandidates = 10
	cfg.Planner.AStar.MaxStates = 5000
	cfg.Planner.AStar.PluginManagerEndpoint = "localhost"
	cfg.Planner.AStar.PluginManagerPort = 33335

	tracer := dummyTracer{}

	var actuatorList []actuators.Actuator
	myPlanner := astar.NewAPlanner(actuatorList, cfg)

	if myPlanner == nil {
		t.Fatal("Planner is nil")
	}
	defer myPlanner.Stop()
	pS := startScaleOutPlugin(tracer, 3334, 33335)
	pR := startRemovePodPlugin(tracer, 3336, 33335)

	s0, g0, profiles := setupTestCase()
	res := myPlanner.CreatePlan(s0, g0, profiles)
	if len(res) != 1 || res[0].Name != "scaleOut" || res[0].Properties.(map[string]int32)["factor"] != 2 {
		klog.Errorf("Planner did not run correctly; result was: %v.", res)
	}

	err := pS.Stop()
	if err != nil {
		klog.ErrorS(err, "error stopping scale_out")
	}
	err = pR.Stop()
	if err != nil {
		klog.ErrorS(err, "error stopping rm_pod")
	}

}

// TestAStarCreatePlan tests the planner including actuators.
func TestAStarCreatePlan(t *testing.T) {
	cfg := common.Config{}
	cfg.Planner.AStar.MaxCandidates = 10
	cfg.Planner.AStar.MaxStates = 5000

	rmPodCfg := scaling.RmPodConfig{}
	rmPodCfg.LookBack = 10
	rmPodCfg.MinPods = 1

	scaleCfg := scaling.ScaleOutConfig{}
	scaleCfg.MaxPods = 128

	tracer := dummyTracer{}

	var actuatorList []actuators.Actuator
	actuatorList = append(actuatorList, scaling.NewScaleOutActuator(nil, tracer, scaleCfg))
	actuatorList = append(actuatorList, scaling.NewRmPodActuator(nil, tracer, rmPodCfg))

	start, goal, profiles := setupTestCase()
	myPlanner := astar.NewAPlanner(actuatorList, cfg)
	res := myPlanner.CreatePlan(start, goal, profiles)
	if len(res) != 1 || res[0].Name != "scaleOut" || res[0].Properties.(map[string]int32)["factor"] != 2 {
		klog.Errorf("Planner did not run correctly; result was: %v.", res)
	}
	myPlanner.Stop()
}
