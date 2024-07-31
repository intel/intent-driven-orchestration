package main

import (
	"flag"
	"fmt"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	pluginsHelper "github.com/intel/intent-driven-orchestration/plugins"

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
		return &scaling.RmPodConfig{}
	})
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	cfg := tmp.(*scaling.RmPodConfig)

	// validate configuration.
	err = pluginsHelper.IsValidGenericConf(cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}
	err = isValidConf(cfg.MinPods, cfg.LookBack)
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
	actuator := scaling.NewRmPodActuator(clusterClient, mt, *cfg)
	signal := pluginsHelper.StartActuatorPlugin(actuator, cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	<-signal
}

func isValidConf(confMinPods, lookBack int) error {
	if confMinPods <= 0 || confMinPods > maxScaleOut {
		return fmt.Errorf("invalid pods number: %d", confMinPods)
	}

	if lookBack <= 0 || lookBack > maxLookBack {
		return fmt.Errorf("invalid lookback value: %d", lookBack)
	}

	return nil
}
