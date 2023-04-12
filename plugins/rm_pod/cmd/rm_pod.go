package main

import (
	"flag"
	"fmt"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"os"
	"os/signal"

	val "github.com/intel/intent-driven-orchestration/plugins"

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

// RmpodPluginHandler represents the actual actuator.
type RmpodPluginHandler struct {
	actuator actuators.Actuator
}

func (s *RmpodPluginHandler) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	klog.V(1).InfoS("Invoked Rmpod Next State Callback")
	return s.actuator.NextState(state, goal, profiles)
}

func (s *RmpodPluginHandler) Perform(state *common.State, plan []planner.Action) {
	klog.V(1).InfoS("Invoked Rmpod Perform Callback")
	s.actuator.Perform(state, plan)
}

func (s *RmpodPluginHandler) Effect(state *common.State, profiles map[string]common.Profile) {
	klog.V(1).InfoS("Invoked Rmpod Effect Callback")
	s.actuator.Effect(state, profiles)
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	tmp, err := common.LoadConfig(config, func() interface{} {
		return &scaling.RmPodConfig{}
	})
	cfg := tmp.(*scaling.RmPodConfig)
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	err = isValidConf(cfg.MinPods)
	if err != nil {
		klog.Fatalf("Error on configuration for actuator: %s", err)
	}
	err = val.IsValidGenericConf(cfg.LookBack, cfg.PluginManagerPort, cfg.Port, "none", "none",
		cfg.Endpoint, cfg.PluginManagerEndpoint, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}

	mt := controller.NewMongoTracer(cfg.MongoEndpoint)
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		klog.Fatalf("Error getting Kubernetes config: %s", err)
	}
	clusterClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating Kubernetes cluster client: %s", err)
	}

	p := &RmpodPluginHandler{
		actuator: scaling.NewRmPodActuator(clusterClient, mt, *cfg),
	}
	stub := plugins.NewActuatorPluginStub(p.actuator.Name(), cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
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

func isValidConf(confMinPods int) error {
	if confMinPods <= 0 || confMinPods > 128 {
		return fmt.Errorf("invalid pods number")
	}
	return nil
}
