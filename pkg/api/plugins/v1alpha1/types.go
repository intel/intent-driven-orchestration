package plugins

import (
	"sync"
	"time"

	protobufs "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1/protobufs"

	"google.golang.org/grpc"

	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
)

// pluginVersion represents a string defining the plugin manager's version.
const pluginVersion = "v1alpha1"

// PInfo plugin info struct
type PInfo struct {
	// Plugin name that uniquely identifies the plugin for the given plugin type.
	Name string
	// Mandatory endpoint location, it usually represents an internal ip to the pod
	// which will handle all plugin requests.
	Endpoint string
	// Plugin service API versions the plugin supports.
	Version string
}

// ActuatorsPluginManager interface for actuator plugins
type ActuatorsPluginManager interface {
	PluginManager
	// Iter thread-safe iterator over registered actuators
	Iter(f func(a actuators.Actuator))
}

// PluginManager interface of an abstract plugin manager which can handle plugin registrations and de-registration
type PluginManager interface {
	// Start starts the grpc server responsible for plugin registrations
	Start() error
	// Stop stops the grpc server responsible for plugin registrations
	Stop() error
	// refreshRegisteredPlugin reconcile callback which checks if plugin connections are still alive and removes all dead connections
	refreshRegisteredPlugin(retries int)
}

type PluginMap map[string]*ActuatorClientStub

// PluginManagerServer implements PluginManager GRPC Server protocol.
type PluginManagerServer struct {
	protobufs.UnimplementedRegistrationServer
	ActuatorsPluginManager
	actuators                []actuators.Actuator
	endpoint                 string
	port                     int
	registeredPlugins        PluginMap
	registeredPluginsRetries map[string]int
	server                   *grpc.Server
	reconcilePeriod          time.Duration
	retries                  int
	wg                       sync.WaitGroup
	mu                       sync.Mutex
	stop                     chan struct{}
}
