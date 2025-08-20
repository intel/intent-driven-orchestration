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

// / maxCPUValue defines the maximum amount of CPU units.
const maxCPUValue = int64(1000 * 1024)

// maxLookBack defines the maximum age a model in the knowledge base can have (1 week)
const maxLookBack = 10080

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
		return &scaling.CPUScaleConfig{}
	})
	if err != nil {
		klog.Fatalf("Error loading configuration for actuator: %s", err)
	}
	cfg := tmp.(*scaling.CPUScaleConfig)

	// validate configuration.
	err = pluginsHelper.IsValidGenericConf(cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort, cfg.MongoEndpoint)
	if err != nil {
		klog.Fatalf("Error on generic configuration for actuator: %s", err)
	}
	err = isValidConf(cfg.PythonInterpreter, cfg.Script, cfg.CPUMax, cfg.CPURounding, cfg.MaxProActiveCPU,
		cfg.BoostFactor, cfg.CPUSafeGuardFactor, cfg.ProActiveLatencyPercentage, cfg.LookBack)
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
	actuator := scaling.NewCPUScaleActuator(clusterClient, mt, *cfg)
	signal := pluginsHelper.StartActuatorPlugin(actuator, cfg.Endpoint, cfg.Port, cfg.PluginManagerEndpoint, cfg.PluginManagerPort)
	<-signal
}

func isValidConf(interpreter, script string, confCPUMax, confCPURounding, confMaxProActiveCPU int64,
	boostFactor, confCPUSafeGuardFactor, configProActiveLatencyPercentage float64, lookBack int) error {
	if !pluginsHelper.IsStrConfigValid(interpreter) {
		return fmt.Errorf("invalid path to python interpreter: %s", interpreter)
	}

	if script != "None" {
		_, err := os.Stat(script)
		if err != nil {
			return fmt.Errorf("invalid script %s", err)
		}
	}

	if confCPUMax <= 0 || confCPUMax > maxCPUValue {
		return fmt.Errorf("invalid cpu numbers: %d", confCPUMax)
	}

	if confCPURounding <= 0 || confCPURounding > 1000 || confCPURounding%10 != 0 {
		return fmt.Errorf("invalid round base: %d", confCPURounding)
	}

	if confMaxProActiveCPU < 0 || confMaxProActiveCPU > confCPUMax {
		return fmt.Errorf("invalid max proactive value: %d", confMaxProActiveCPU)
	}

	if boostFactor < 0.0 || boostFactor > 10.0 {
		return fmt.Errorf("invalid boost factor - needs to be in range of 0-10; was: %f", boostFactor)
	}

	if confCPUSafeGuardFactor <= 0 || confCPUSafeGuardFactor > 1 {
		return fmt.Errorf("invalid safeguard factor: %f", confCPUSafeGuardFactor)
	}

	if configProActiveLatencyPercentage < 0 || configProActiveLatencyPercentage > 1 {
		return fmt.Errorf("invalid fraction value for proactive latency: %f", configProActiveLatencyPercentage)
	}

	if lookBack <= 0 || lookBack > maxLookBack {
		return fmt.Errorf("invalid lookback value: %d", lookBack)
	}

	return nil
}
