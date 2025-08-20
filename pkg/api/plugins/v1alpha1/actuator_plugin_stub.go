package plugins

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	protobufs "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1/protobufs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
)

// stubNextStateFunc nextState function type for stub callbacks
type stubNextStateFunc func(*common.State, *common.State, map[string]common.Profile) ([]common.State, []float64, []planner.Action)

// stubPerformFunc perform function type for stub callbacks
type stubPerformFunc func(*common.State, []planner.Action)

// stubEffectFunc effect function type for stub callbacks
type stubEffectFunc func(*common.State, map[string]common.Profile)

// defaultNextStateFunc default nextState handler callback
func defaultNextStateFunc(*common.State, *common.State, map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	return []common.State{}, []float64{}, []planner.Action{}
}

// defaultPerformFunc default perform handler callback
func defaultPerformFunc(*common.State, []planner.Action) {
}

// defaultEffectFunc default effect handler callback
func defaultEffectFunc(*common.State, map[string]common.Profile) {
}

// ActuatorPluginStub Stub implementation for ActuatorPlugin. Plugins have to instantiate this stub and
// set specific function callbacks. An actuator needs to support nextState, perform and effect callbacks
type ActuatorPluginStub struct {
	protobufs.UnimplementedActuatorPluginServer
	server                *grpc.Server
	name                  string
	version               string
	endpoint              string
	port                  int
	pluginManagerEndpoint string
	pluginManagerPort     int
	retries               int
	nextStateFunc         stubNextStateFunc
	performFunc           stubPerformFunc
	effectFunc            stubEffectFunc

	stop chan interface{}
	wg   sync.WaitGroup
}

// NewActuatorPluginStub creates a new actuator stub for a user defined plugin manager endpoint.
func NewActuatorPluginStub(name string, endpoint string, port int, serverEndpoint string, serverPort int) *ActuatorPluginStub {
	return &ActuatorPluginStub{
		name:                  name,
		version:               pluginVersion,
		endpoint:              endpoint,
		port:                  port,
		pluginManagerEndpoint: serverEndpoint,
		pluginManagerPort:     serverPort,
		retries:               3,
		nextStateFunc:         defaultNextStateFunc,
		performFunc:           defaultPerformFunc,
		effectFunc:            defaultEffectFunc,
		stop:                  make(chan interface{}),
	}
}

// Start starts the grpc server for the actuator plugin stub
func (s *ActuatorPluginStub) Start() error {
	sock, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}

	s.wg.Add(1)
	s.server = grpc.NewServer([]grpc.ServerOption{}...)
	protobufs.RegisterActuatorPluginServer(s.server, s)

	go func() {
		defer s.wg.Done()
		err = s.server.Serve(sock)
		if err != nil {
			klog.ErrorS(err, "Error while serving actuator socket")
		}
	}()

	var lastDialErr error
	// TODO: make configurable.
	err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 10*time.Second, true, func(_ context.Context) (bool, error) {
		var conn *grpc.ClientConn
		// nolint:staticcheck // SA1019: grpc.Dial is deprecated — but supported in 1.0; for GRPC 2.0 we'll need to check if the connection is ready.
		conn, lastDialErr = grpc.Dial(fmt.Sprintf("%s:%d", s.endpoint, s.port), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if lastDialErr != nil {
			// nolint:nilerr
			return false, nil
		}
		conn.Close()
		return true, nil
	})

	if err != nil {
		klog.ErrorS(err, "Error while checking server socket availability.")
	}

	if lastDialErr != nil {
		return lastDialErr
	}

	klog.Infof("Actuator %s: starting to serve on endpoint: %s:%d.", s.name, s.endpoint, s.port)
	return nil
}

// Stop stops the gRPC server. Can be called without a prior Start
// and more than once. Not safe to be called concurrently by different
// goroutines!
func (s *ActuatorPluginStub) Stop() error {
	if s.server == nil {
		return nil
	}
	s.server.Stop()
	s.wg.Wait()
	s.server = nil
	close(s.stop) // This prevents re-starting the server.
	klog.Infof("Stopping plugin stub for %s.", s.name)
	return nil
}

// Register registers the actuator plugin for the given name with ido controller.
func (s *ActuatorPluginStub) Register() error {
	klog.Infof("Actuator %s: performing plugin registration at %s:%d.", s.name, s.pluginManagerEndpoint, s.pluginManagerPort)
	if s.port <= 0 || s.port > 65535 || s.pluginManagerPort <= 0 || s.pluginManagerPort > 65535 {
		return fmt.Errorf("failed. Both ports need to be in a valid range: %d - %d", s.port, s.pluginManagerPort)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // TODO: make configurable.
	defer cancel()
	// nolint:staticcheck // SA1019: grpc.Dial is deprecated — but supported in 1.0; for GRPC 2.0 we'll need to check if the connection is ready.
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("%s:%d", s.pluginManagerEndpoint, s.pluginManagerPort), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		klog.ErrorS(err, "Cannot establish a connection to the plugin manager.")
	}
	retries := s.retries
	for retries > 0 && err != nil && (conn == nil || conn.GetState() != connectivity.Ready) {
		time.Sleep(5 * time.Second) // TODO: make configurable.
		// nolint:staticcheck // SA1019: grpc.Dial is deprecated — but supported in 1.0; for GRPC 2.0 we'll need to check if the connection is ready.
		conn, err = grpc.DialContext(ctx, fmt.Sprintf("%s:%d", s.pluginManagerEndpoint, s.pluginManagerPort), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			klog.ErrorS(err, "Cannot establish connection to the plugin manager.")
		}
		retries--
	}
	if conn == nil || conn.GetState() != connectivity.Ready {
		return fmt.Errorf("failed to establish a connection to plugin manager after %d retries", s.retries)
	}
	defer conn.Close()
	client := protobufs.NewRegistrationClient(conn)
	req := protobufs.RegisterRequest{
		PInfo: &protobufs.PluginInfo{
			Type:              protobufs.PluginType_ACTUATOR,
			Name:              s.name,
			Endpoint:          fmt.Sprintf("%s:%d", s.endpoint, s.port),
			SupportedVersions: s.version,
		},
	}
	resp, err := client.Register(context.Background(), &req)
	if err == nil && resp.Error != "" {
		return fmt.Errorf("server side registration error: %s", resp.Error)
	}
	return err
}

// SetNextStateFunc sets the NextState function callback
func (s *ActuatorPluginStub) SetNextStateFunc(f stubNextStateFunc) {
	s.nextStateFunc = f
}

// SetPerformFunc sets the Perform function callback
func (s *ActuatorPluginStub) SetPerformFunc(f stubPerformFunc) {
	s.performFunc = f
}

// SetEffectFunc sets the Effect function callback
func (s *ActuatorPluginStub) SetEffectFunc(f stubEffectFunc) {
	s.effectFunc = f
}

// toState state type conversion from grpc to internal datatype
func toState(s *protobufs.State) *common.State {
	gs := common.State{
		Intent: common.Intent{
			Key:        s.Intent.Key,
			Priority:   s.Intent.Priority,
			TargetKey:  s.Intent.TargetKey,
			TargetKind: s.Intent.TargetKind,
			Objectives: s.Intent.Objectives,
		},
		CurrentPods: make(map[string]common.PodState),
		CurrentData: make(map[string]map[string]float64),
		Resources:   make(map[string]int64),
		Annotations: make(map[string]string),
	}

	for k, v := range s.CurrentPods {
		gs.CurrentPods[k] = common.PodState{
			Availability: v.Availability,
			NodeName:     v.NodeName,
			State:        v.State,
			QoSClass:     v.QosClass,
		}
	}
	for k, v := range s.CurrentData {
		gs.CurrentData[k] = v.Data
	}
	for k, v := range s.Resources {
		gs.Resources[k] = v
	}
	for k, v := range s.Annotations {
		gs.Annotations[k] = v
	}
	return &gs
}

// toProfiles profiles type conversion from grpc to internal datatype
func toProfiles(profiles map[string]*protobufs.Profile) map[string]common.Profile {
	r := map[string]common.Profile{}
	for k, v := range profiles {
		r[k] = common.Profile{
			Key:         v.Key,
			ProfileType: common.ProfileTypeFromText(v.ProfileType.String()),
			Minimize:    v.Minimize,
		}
	}
	return r
}

// toActions actions type conversion from grpc to internal datatype
func toActions(plan []*protobufs.Action) []planner.Action {
	var res []planner.Action
	for _, a := range plan {
		var p interface{}
		if a.Properties.Type == protobufs.PropertyType_INT_PROPERTY {
			p = a.Properties.IntProperties
		} else {
			p = a.Properties.StrProperties
		}
		res = append(res, planner.Action{
			Name:       a.Name,
			Properties: p,
		})
	}
	return res

}

// getNextStateResponseServer generates next state response
func getNextStateResponseServer(states []common.State, utilities []float64, actions []planner.Action) *protobufs.NextStateResponse {
	return &protobufs.NextStateResponse{
		States:    toGrpcStates(states),
		Utilities: utilities,
		Actions:   toGrpcActions(actions),
	}
}

// NextState grpc callback for the nextState function of pluggable Actuators
func (s *ActuatorPluginStub) NextState(stream protobufs.ActuatorPlugin_NextStateServer) error {
	klog.V(3).InfoS("NextState GRPC call", "request", stream)
	for {
		r, err := stream.Recv()
		if err != nil {
			return err
		}
		response := getNextStateResponseServer(s.nextStateFunc(toState(r.State), toState(r.Goal), toProfiles(r.Profiles)))
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

// Perform grpc callback for the perform function of pluggable Actuators
func (s *ActuatorPluginStub) Perform(_ context.Context, r *protobufs.PerformRequest) (*protobufs.Empty, error) {
	klog.V(3).InfoS("Perform GRPC call", "request", r)
	s.performFunc(toState(r.State), toActions(r.Plan))
	return &protobufs.Empty{}, nil
}

// Effect grpc callback for the Effect function of pluggable Actuators
func (s *ActuatorPluginStub) Effect(_ context.Context, r *protobufs.EffectRequest) (*protobufs.Empty, error) {
	klog.V(3).InfoS("Effect GRPC call", "request", r)
	s.effectFunc(toState(r.State), toProfiles(r.Profiles))
	return &protobufs.Empty{}, nil
}
