package tests

import (
	"flag"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/energy"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/scaling"
	"k8s.io/klog/v2"
)

func init() {
	var fs flag.FlagSet
	klog.InitFlags(&fs)
	err := fs.Set("v", "2")
	if err != nil {
		klog.Errorf("Ohoh unable to set log level: %v.", err)
	}
}

func TestPowerForSanity(t *testing.T) {
	defaultsConfig, err1 := common.LoadConfig("traces/defaults.json", func() interface{} {
		return &common.Config{}
	})
	cpuScaleConfig, err2 := common.LoadConfig("traces/cpu_scale.json", func() interface{} {
		return &scaling.CPUScaleConfig{}
	})
	if err1 != nil || err2 != nil {
		t.Errorf("Could not load config files!")
	}

	cfg := energy.PowerActuatorConfig{
		Prediction:        "../.././pkg/planner/actuators/energy/analytics/test_predict.py",
		Analytics:         "../.././pkg/planner/actuators/energy/analytics/test_analytics.py",
		PowerProfiles:     []string{"None", "power.intel.com/balance-power", "power.intel.com/balance-performance", "power.intel.com/performance"},
		PythonInterpreter: "python3",
		RenewableLimit:    0.75,
		StepDown:          2,
	}
	registry := map[string]actuatorSetup{
		"NewCPUScaleActuator": {scaling.NewCPUScaleActuator, *cpuScaleConfig.(*scaling.CPUScaleConfig)},
		"NewPowerActuator":    {energy.NewPowerActuator, cfg},
	}

	var tests = []testEnvironment{
		{name: "power_efficiency", effectsFilename: "traces/trace_2/effects.json", eventsFilename: "traces/trace_2/events.json", defaults: defaultsConfig.(*common.Config), actuators: registry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTrace(tt, t, cleanUpWithPower)
		})
	}
}

// cleanUp makes sure we stop the prediction service.
func cleanUpWithPower(actuators []actuators.Actuator) {
	for _, actuator := range actuators {
		klog.Infof("Cleaning up actuator '%s'...", actuator.Name())
		switch v := actuator.(type) {
		case *energy.PowerActuator:
			err := v.Cmd.Process.Kill()
			if err != nil {
				klog.Errorf("Could not cleanup actuator: %v.", err)
			}
		default:
			klog.Infof("Actuator without cleanup needs: %v.", v)
		}
	}
}
