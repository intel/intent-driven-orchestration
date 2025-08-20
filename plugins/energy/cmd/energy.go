package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/energy"
	pluginsHelper "github.com/intel/intent-driven-orchestration/plugins"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"k8s.io/klog/v2"
)

var (
	kubeConfig string
	config     string
)

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "Path to a kube config file.")
	flag.StringVar(&config, "config", "", "Path to configuration file.")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	tmp, err := common.LoadConfig(config, func() interface{} {
		return &energy.PowerActuatorConfig{}
	})
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	cfg := tmp.(*energy.PowerActuatorConfig)

	// validate configuration.
	err = pluginsHelper.IsValidGenericConf(cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}
	err = isValidConf(cfg.PythonInterpreter, cfg.Prediction, cfg.Analytics, cfg.StepDown, cfg.RenewableLimit, cfg.PowerProfiles)
	if err != nil {
		klog.Fatalf("Error on configuration for actuator: %s", err)
	}

	// get K8s config.
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		klog.Fatalf("Error getting Kubernetes config: %s", err)
	}
	clusterClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating Kubernetes cluster client: %s", err)
	}

	// once configuration is ready & valid start the plugin mechanism.
	mt := controller.NewMongoTracer(cfg.MongoEndpoint)
	actuator := energy.NewPowerActuator(clusterClient, mt, *cfg)
	signal := pluginsHelper.StartActuatorPlugin(actuator, cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	<-signal
}

func isValidConf(interpreter, analytics, prediction string, stepDown int, renewableLimit float64, profiles []string) error {
	if !pluginsHelper.IsStrConfigValid(interpreter) {
		return fmt.Errorf("invalid path to python interpreter: %s", interpreter)
	}

	if analytics != "None" {
		_, err := os.Stat(analytics)
		if err != nil {
			return fmt.Errorf("invalid analytics script %s", err)
		}
	}

	if prediction != "None" {
		_, err := os.Stat(prediction)
		if err != nil {
			return fmt.Errorf("invalid prediction script %s", err)
		}
	}

	if len(profiles) < 2 {
		return fmt.Errorf("invalid number of profiles - expect at least 2: %d", len(profiles))
	}

	if stepDown < 1 || stepDown >= len(profiles) {
		return fmt.Errorf("step down needs to be in the range of 1 to %d - was: %d", len(profiles), stepDown)
	}

	if renewableLimit <= 0 {
		return fmt.Errorf("renewable energy limit needs to be bigger thean 0 - was: %f", renewableLimit)
	}

	return nil
}
