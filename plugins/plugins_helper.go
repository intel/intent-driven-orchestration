package pluginshelper

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	plugins "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
	"k8s.io/klog/v2"
)

// StartActuatorPlugin starts the necessary Stubs and registers the plugin with the plugin manager.
func StartActuatorPlugin(actuator actuators.Actuator, endpoint string, port int, serverEndpoint string, serverPort int) chan os.Signal {
	stub := plugins.NewActuatorPluginStub(actuator.Name(), endpoint, port, serverEndpoint, serverPort)
	stub.SetNextStateFunc(actuator.NextState)
	stub.SetPerformFunc(actuator.Perform)
	stub.SetEffectFunc(actuator.Effect)
	err := stub.Start()
	if err != nil {
		klog.Fatalf("Error starting plugin server: %s", err)
	}
	err = stub.Register()
	if err != nil {
		klog.Fatalf("Error registering plugin: %s", err)
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		err = stub.Stop()
		if err != nil {
			klog.Fatalf("Error stopping plugin server: %s", err)
		}
	}()
	return signalChan
}

// IsValidGenericConf checks if a set of generic configuration fields are valid.
func IsValidGenericConf(endpoint string, port int, pluginManagerEndpoint string, pluginManagerPort int, mongo string) error {
	if !isPortNumValid(port) || !isPortNumValid(pluginManagerPort) {
		return fmt.Errorf("invalid port value")
	}

	_, err := url.ParseRequestURI(mongo)
	if err != nil {
		return fmt.Errorf("invalid uri: %s", err)
	}

	if !IsStrConfigValid(endpoint) || !IsStrConfigValid(pluginManagerEndpoint) {
		return fmt.Errorf("invalid endpoint value")
	}

	return nil
}

// IsStrConfigValid checks if string property of a configuration is valid.
func IsStrConfigValid(str string) bool {
	if str != "" && len(str) < 101 {
		return true
	}
	return false
}

// isPortNumValid checks if a port definition is > 0 and smaller 65536.
func isPortNumValid(num int) bool {
	if num > 0 && num < 65536 {
		return true
	}
	return false
}
