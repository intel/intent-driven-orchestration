package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	pluginsHelper "github.com/intel/intent-driven-orchestration/plugins"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/scaling"

	"k8s.io/klog/v2"
)

// maxLookBack defines the maximum age a model in the knowledge base can have (1 week)
const maxLookBack = 10080

// maxScaleOut defines the maximum number of replicas.
const maxScaleOut = 128

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
		return &scaling.ScaleOutConfig{}
	})
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	cfg := tmp.(*scaling.ScaleOutConfig)

	// validate configuration.
	err = pluginsHelper.IsValidGenericConf(cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}
	err = isValidConf(cfg.PythonInterpreter, cfg.Script, cfg.MaxPods, cfg.MaxProActiveScaleOut, cfg.LookBack, cfg.ProActiveLatencyFactor)
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
	actuator := scaling.NewScaleOutActuator(clusterClient, mt, *cfg)
	signal := pluginsHelper.StartActuatorPlugin(actuator, cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	<-signal
}

func isValidConf(interpreter, script string, confMaxPods, confMaxProactiveScaleOut, lookBack int, confProActiveLatencyFactor float64) error {
	if !pluginsHelper.IsStrConfigValid(interpreter) {
		return fmt.Errorf("invalid path to python interpreter: %s", interpreter)
	}

	if script != "None" {
		_, err := os.Stat(script)
		if err != nil {
			return fmt.Errorf("invalid script %s", err)
		}
	}

	if confMaxPods <= 0 || confMaxPods > maxScaleOut {
		return fmt.Errorf("invalid pods number: %d", confMaxPods)
	}

	if confMaxProactiveScaleOut < 0 || confMaxProactiveScaleOut > confMaxPods {
		return fmt.Errorf("invalid max proactive value: %d", confMaxProactiveScaleOut)
	}

	if lookBack <= 0 || lookBack > maxLookBack {
		return fmt.Errorf("invalid lookback value: %d", lookBack)
	}

	if confProActiveLatencyFactor < 0 || confProActiveLatencyFactor > 1 {
		return fmt.Errorf("invalid fraction value for proactive latency: %f", confProActiveLatencyFactor)
	}

	return nil
}
