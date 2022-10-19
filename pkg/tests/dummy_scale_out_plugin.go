package tests

import (
	plugins "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/controller"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/scaling"

	"k8s.io/klog/v2"
)

type DummyScaleOutPluginHandler struct {
	actuator actuators.Actuator
}

func (s *DummyScaleOutPluginHandler) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	return s.actuator.NextState(state, goal, profiles)
}

func (s *DummyScaleOutPluginHandler) Perform(state *common.State, plan []planner.Action) {
	s.actuator.Perform(state, plan)
}

func (s *DummyScaleOutPluginHandler) Effect(state *common.State, profiles map[string]common.Profile) {
	s.actuator.Effect(state, profiles)
}

// startScaleOutPlugin initiates a scale_ou actuator.
func startScaleOutPlugin(tracer controller.Tracer, port int, pluginManagerPort int) *plugins.ActuatorPluginStub {
	cfg := scaling.ScaleOutConfig{
		MaxPods: 128,
	}
	p := &DummyScaleOutPluginHandler{
		actuator: scaling.NewScaleOutActuator(nil, tracer, cfg),
	}
	stub := plugins.NewActuatorPluginStub("scale_out", "localhost", port, "localhost", pluginManagerPort)
	stub.SetNextStateFunc(p.NextState)
	stub.SetPerformFunc(p.Perform)
	stub.SetEffectFunc(p.Effect)
	err := stub.Start()
	if err != nil {
		klog.Fatalf("Error starting plugin: %s", err)
		return nil
	}
	err = stub.Register()
	if err != nil {
		klog.Fatalf("Error registering plugin: %s", err)
		return nil
	}
	return stub
}
