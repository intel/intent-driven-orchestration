package plugins

import (
	"context"
	"fmt"
	"net"
	"time"

	protobufs "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1/protobufs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
)

// NewPluginManagerServer initializes new plugin manager server.
func NewPluginManagerServer(actuators []actuators.Actuator, endpoint string, port int) *PluginManagerServer {
	return &PluginManagerServer{
		actuators:                actuators,
		endpoint:                 endpoint,
		port:                     port,
		registeredPlugins:        make(PluginMap),
		registeredPluginsRetries: make(map[string]int),
		stop:                     make(chan struct{}),
		reconcilePeriod:          5 * time.Second, // TODO: make configurable.
		retries:                  3,               // TODO: make configurable.
	}
}

// Register registration callback triggered when the server received a new register rpc call from a plugin
func (pm *PluginManagerServer) Register(_ context.Context, r *protobufs.RegisterRequest) (*protobufs.RegistrationStatusResponse, error) {
	klog.Infof("Received plugin registration for plugin name: %s with endpoint: %s.", r.PInfo.Name, r.PInfo.Endpoint)
	resp := &protobufs.RegistrationStatusResponse{
		PluginRegistered: false,
		Error:            "",
	}

	if r.PInfo.SupportedVersions != pluginVersion {
		resp.Error = fmt.Sprintf("Unsupported plugin version: %s.", r.PInfo.SupportedVersions)
		return resp, nil
	}
	// we do not allow plugin registration with the same name
	ok := false
	pm.mu.Lock()
	_, ok = pm.registeredPlugins[r.PInfo.Name]
	pm.mu.Unlock()
	if !ok {
		aClientStub, err := newActuatorClientStub(r.PInfo, pm.retries)
		if err != nil {
			resp.Error = fmt.Sprintf("Actuator Client Stub Error: %s.", err)
			return resp, nil
		}
		pm.mu.Lock()
		pm.registeredPlugins[r.PInfo.Name] = aClientStub
		pm.mu.Unlock()
		resp.PluginRegistered = true

	} else {
		klog.Warningf("Plugin %s is already registered.", r.PInfo.Name)
		resp.Error = "Plugin is already registered."
	}
	return resp, nil
}

// refreshRegisteredPlugin checks if registered plugins have healthy connection, if not they are removed from the registered list
func (pm *PluginManagerServer) refreshRegisteredPlugin(retries int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	var toBeDeleted []string
	for pName, a := range pm.registeredPlugins {
		if a.clientConn.GetState() != connectivity.Ready {
			_, ok := pm.registeredPluginsRetries[pName]
			if !ok {
				pm.registeredPluginsRetries[pName] = 0
			}
			if pm.registeredPluginsRetries[pName] >= retries {
				toBeDeleted = append(toBeDeleted, a.pluginInfo.Name)
			}
			pm.registeredPluginsRetries[pName]++
		} else {
			pm.registeredPluginsRetries[pName] = 0
		}
	}
	klog.V(1).Infof("Active plugins vs to be removed plugins: %d/%d", len(pm.registeredPlugins), len(toBeDeleted))
	for _, k := range toBeDeleted {
		delete(pm.registeredPlugins, k)
		delete(pm.registeredPluginsRetries, k)
	}
}

// Iter a thread-safe iterator over registered actuators which can apply a functor f
func (pm *PluginManagerServer) Iter(f func(a actuators.Actuator)) {
	for _, a := range pm.actuators {
		f(a)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, a := range pm.registeredPlugins {
		f(a)
	}
}

// Start starts the grpc server for the actuator
func (pm *PluginManagerServer) Start() error {
	sock, err := net.Listen("tcp", fmt.Sprintf(":%d", pm.port))
	if err != nil {
		return err
	}

	pm.wg.Add(1)
	pm.server = grpc.NewServer([]grpc.ServerOption{}...)
	protobufs.RegisterRegistrationServer(pm.server, pm)

	go func() {
		defer pm.wg.Done()
		err = pm.server.Serve(sock)
		if err != nil {
			klog.ErrorS(err, "Error during serving.")
		}
	}()
	go wait.Until(func() { pm.refreshRegisteredPlugin(pm.retries) }, pm.reconcilePeriod, pm.stop)
	var lastDialErr error
	err = wait.PollImmediate(1*time.Second, 10*time.Second, func() (bool, error) { // TODO: make configurable.
		var conn *grpc.ClientConn
		conn, lastDialErr = grpc.Dial(fmt.Sprintf("localhost:%d", pm.port), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if lastDialErr != nil {
			// nolint:nilerr
			return false, nil
		}
		conn.Close()
		return true, nil
	})
	if err != nil {
		klog.ErrorS(err, "Timeout occurred while checking plugin manager server connection health.")
	}
	if lastDialErr != nil {
		return lastDialErr
	}

	klog.Infof("Starting to serve on endpoint %s:%d.", pm.endpoint, pm.port)
	return nil
}

// Stop stops the plugin manager registration server and closes all outgoing plugin connections
func (pm *PluginManagerServer) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, p := range pm.registeredPlugins {
		p.stop()
	}

	if pm.server == nil {
		return nil
	}
	pm.server.Stop()
	pm.wg.Wait()
	close(pm.stop)
	pm.server = nil
	return nil
}
