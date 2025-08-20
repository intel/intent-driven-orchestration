package main

import (
	"flag"
	"io"
	"os"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/controller"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
	"github.com/intel/intent-driven-orchestration/pkg/planner/astar"

	kubeInformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	genClient "github.com/intel/intent-driven-orchestration/pkg/generated/clientset/versioned"
	genInformer "github.com/intel/intent-driven-orchestration/pkg/generated/informers/externalversions"
)

var (
	kubeConfig string
	config     string
)

func main() {
	klog.InitFlags(nil)
	klog.Info("Hello from your friendly autonomous planning component...")

	stopper := make(chan struct{})

	// config stuff.
	flag.Parse()
	k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		klog.Fatalf("Error loading Kubernetes config: %s", err)
	}
	cfg, err := common.ParseConfig(config)
	if err != nil {
		klog.Fatalf("Error loading planner config: %v", err)
	}

	// set logFile
	if cfg.Generic.LogFile != "" {
		err := flag.Set("logtostderr", "false")
		if err != nil {
			klog.Fatalf("Error setting flag logtostderr: %v", err)
		}
		err = flag.Set("alsologtostderr", "true")
		if err != nil {
			klog.Fatalf("Error setting flag alsologtostderr: %v", err)
		}

		logFile, err := os.OpenFile(cfg.Generic.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			klog.Fatalf("Failed to open log file: %v", err)
		}
		defer logFile.Close()

		multiWriter := io.MultiWriter(os.Stdout, logFile)
		klog.SetOutput(multiWriter)
		defer func() {
			klog.Flush()
		}()
		klog.Infof("Successfuly added to klog output the log file: %s", cfg.Generic.LogFile)
	}

	// K8s genClient setup
	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		klog.Fatalf("Error getting k8s-client config: %s", err)
	}
	podInformerFactory := kubeInformers.NewSharedInformerFactory(k8sClient, time.Second*time.Duration(cfg.Controller.InformerTimeout))

	crdClient, err := genClient.NewForConfig(k8sConfig)
	if err != nil {
		klog.Fatalf("Error getting gen-client config: %s", err)
	}
	informerFactory := genInformer.NewSharedInformerFactory(crdClient, time.Second*time.Duration(cfg.Controller.InformerTimeout))

	// The planning algorithm.
	// TODO: make all of this more configurable.
	var actuatorList []actuators.Actuator
	// UNCOMMENT to add your local actuators.
	// tracer := controller.NewMongoTracer(cfg.Generic.MongoEndpoint)
	// actuatorList = append(actuatorList, scaling.NewScaleOutActuator(k8sClient, tracer))
	// actuatorList = append(actuatorList, scaling.NewRmPodActuator(k8sClient, tracer))
	// actuatorList = append(actuatorList, platform.NewRdtActuator(k8sClient, tracer))
	planner := astar.NewAPlanner(actuatorList, cfg)
	defer planner.Stop()

	// This is the main controller.
	tracer := controller.NewMongoTracer(cfg.Generic.MongoEndpoint)
	c := controller.NewController(cfg, tracer, k8sClient, podInformerFactory.Core().V1().Pods())
	c.SetPlanner(planner)

	// 1/3 bring up the monitor for the KPIProfiles.
	profileMonitor := controller.NewKPIProfileMonitor(
		cfg.Monitor,
		crdClient,
		informerFactory.Ido().V1alpha1().KPIProfiles(),
		c.UpdateProfile())
	go profileMonitor.Run(cfg.Monitor.Profile.Workers, stopper)

	// 2/3 bring up the monitor for the intents.
	intentMonitor := controller.NewIntentMonitor(
		crdClient,
		informerFactory.Ido().V1alpha1().Intents(),
		c.UpdateIntent())
	go intentMonitor.Run(cfg.Monitor.Intent.Workers, stopper)

	// 3/3 bring up the monitor for the PODs.
	podMonitor := controller.NewPodMonitor(
		k8sClient,
		podInformerFactory.Core().V1().Pods(),
		c.UpdatePodError())
	go podMonitor.Run(cfg.Monitor.Pod.Workers, stopper)

	// run the actual overall logic.
	c.Run(cfg.Controller.Workers, stopper)
	informerFactory.Start(stopper)
	podInformerFactory.Start(stopper)

	// TODO: implement proper stop signal handler.
	<-stopper
}

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "Path to a kube config file.")
	flag.StringVar(&config, "config", "", "Path to configuration file.")
}
