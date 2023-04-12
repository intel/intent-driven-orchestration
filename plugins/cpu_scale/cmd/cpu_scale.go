package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"k8s.io/client-go/rest"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	val "github.com/intel/intent-driven-orchestration/plugins"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	plugins "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/scaling"

	"k8s.io/klog/v2"
)

var (
	kubeConfig string
	config     string
)

type CPUScalePluginHandler struct {
	actuator actuators.Actuator
}

func (s *CPUScalePluginHandler) NextState(state *common.State, goal *common.State,
	profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	klog.InfoS("From plugin: Invoked CPUScale Next State Callback")
	return s.actuator.NextState(state, goal, profiles)
}

func (s *CPUScalePluginHandler) Perform(state *common.State, plan []planner.Action) {
	klog.InfoS("From plugin: Invoked CPUScale Perform Callback")
	s.actuator.Perform(state, plan)
}

func (s *CPUScalePluginHandler) Effect(state *common.State, profiles map[string]common.Profile) {
	klog.InfoS("From plugin: Invoked CPUScale Effect Callback")
	s.actuator.Effect(state, profiles)
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	tmp, err := common.LoadConfig(config, func() interface{} {
		return &scaling.CPUScaleConfig{}
	})

	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}

	cfg := tmp.(*scaling.CPUScaleConfig)

	err = isValidConf(cfg.CPUMax, cfg.CPURounding, cfg.MaxProActiveCPU,
		cfg.CPUSafeGuardFactor, cfg.ProActiveLatencyPercentage)
	if err != nil {
		klog.Fatalf("Error on configuration for actuator: %s", err)
	}

	err = val.IsValidGenericConf(cfg.LookBack, cfg.PluginManagerPort, cfg.Port,
		cfg.PythonInterpreter, cfg.Script, cfg.Endpoint, cfg.PluginManagerEndpoint, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}

	mt := controller.NewMongoTracer(cfg.MongoEndpoint)
	var config *rest.Config
	config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)

	if err != nil {
		klog.Fatalf("Error getting Kubernetes config: %s", err)
	}

	var clusterClient *kubernetes.Clientset
	clusterClient, err = kubernetes.NewForConfig(config)

	if err != nil {
		klog.Fatalf("Error creating Kubernetes cluster client: %s", err)
	}

	p := &CPUScalePluginHandler{
		actuator: scaling.NewCPUScaleActuator(clusterClient, mt, *cfg),
	}
	stub := plugins.NewActuatorPluginStub(p.actuator.Name(), cfg.Endpoint, cfg.Port,
		cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	stub.SetNextStateFunc(p.NextState)
	stub.SetPerformFunc(p.Perform)
	stub.SetEffectFunc(p.Effect)
	err = stub.Start()

	if err != nil {
		klog.Fatalf("Error starting plugin server: %s", err)
	}

	err = stub.Register()

	if err != nil {
		klog.Fatalf("Error registering plugin: %s", err)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan

	err = stub.Stop()

	if err != nil {
		klog.Fatalf("Error stopping plugin server: %s", err)
	}
}

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "Path to a kube config file.")
	flag.StringVar(&config, "config", "", "Path to configuration file.")
}

func isValidConf(confCPUMax, confCPURounding, confMaxProActiveCPU int64,
	confCPUSafeGuardFactor, configProActiveLatencyPercentage float64) error {
	if confCPUMax <= 0 || confCPUMax > int64(1000*1024) {
		return fmt.Errorf("invalid cpu numbers")
	}

	if confCPURounding <= 0 || confCPURounding > 1000 || confCPURounding%10 != 0 {
		return fmt.Errorf("invalid round base")
	}

	if confCPUSafeGuardFactor <= 0 || confCPUSafeGuardFactor > 1 {
		return fmt.Errorf("invalid safeguard factor")
	}

	if confMaxProActiveCPU < 0 || confMaxProActiveCPU > confCPUMax {
		return fmt.Errorf("invalid max proactive value")
	}

	if configProActiveLatencyPercentage < 0 || configProActiveLatencyPercentage > 1 {
		return fmt.Errorf("invalid fraction value for proactive latency")
	}

	return nil
}
