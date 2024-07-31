package main

import (
	"flag"
	"fmt"
	"os"

	pluginsHelper "github.com/intel/intent-driven-orchestration/plugins"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	val "github.com/intel/intent-driven-orchestration/plugins"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/platform"

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
		return &platform.RdtConfig{}
	})
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	cfg := tmp.(*platform.RdtConfig)

	// validate configuration.
	err = val.IsValidGenericConf(cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}
	err = isValidConf(cfg.Interpreter, cfg.Analytics, cfg.Prediction, cfg.Options)
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
	actuator := platform.NewRdtActuator(clusterClient, mt, *cfg)
	signal := pluginsHelper.StartActuatorPlugin(actuator, cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	<-signal
}

func isValidConf(interpreter, analyticsScript, predictionScript string, options []string) error {
	// TODO: implement!
	if !val.IsStrConfigValid(interpreter) {
		return fmt.Errorf("invalid path to python interpreter: %s", interpreter)
	}

	if analyticsScript != "None" {
		_, err := os.Stat(analyticsScript)
		if err != nil {
			return fmt.Errorf("invalid analytics script %s", err)
		}
	}

	if predictionScript != "None" {
		_, err := os.Stat(predictionScript)
		if err != nil {
			return fmt.Errorf("invalid prediction script %s", err)
		}
	}

	if len(options) == 0 {
		return fmt.Errorf("not enough options defined: %v", options)
	}

	return nil
}
