//TODO copyright

package plugins

import (
	"context"
	"net"
	"testing"

	protobufs "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1/protobufs"

	"k8s.io/klog/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
)

type PluginManagerMock struct {
	protobufs.UnimplementedRegistrationServer
	ActuatorsPluginManager
	mock.Mock
}

func (m *PluginManagerMock) Register(ctx context.Context, req *protobufs.RegisterRequest) (*protobufs.RegistrationStatusResponse, error) {
	args := m.Called(ctx, req)
	resp := &protobufs.RegistrationStatusResponse{
		PluginRegistered: true,
		Error:            "",
	}
	return resp, args.Error(0)
}

// Creates a buffered conn grpc server for testing.
func newTestPluginManager(ctx context.Context) (protobufs.RegistrationClient, func(), *PluginManagerMock) {
	buffer := 1024 * 1024
	listener := bufconn.Listen(buffer)
	s := grpc.NewServer()
	m := PluginManagerMock{}
	protobufs.RegisterRegistrationServer(s, &m)
	go func() {
		err := s.Serve(listener)
		if err != nil {
			if err == grpc.ErrServerStopped {
				klog.Info("Server stopped")
			}
			klog.Errorf("Server serve error: %v", err)
		}
	}()
	// nolint:staticcheck // SA1019: grpc.Dial is deprecated — but supported in 1.0; for GRPC 2.0 we'll need to check if the connection is ready.
	conn, _ := grpc.DialContext(ctx, "", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	closer := func() {
		err := listener.Close()
		if err != nil {
			klog.Error(err)
		}
		s.GracefulStop()
	}

	client := protobufs.NewRegistrationClient(conn)

	return client, closer, &m
}

func TestNewPluginManager(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 55555)
	assert.NotNil(t, pm)
}

func TestSuccessfulPluginRegistration(t *testing.T) {
	ctx := context.Background()
	pluginManager, closer, mDaemon := newTestPluginManager(ctx)
	regReq := protobufs.RegisterRequest{
		PInfo: &protobufs.PluginInfo{
			Type:              protobufs.PluginType_ACTUATOR,
			SupportedVersions: "v1alpha1",
			Endpoint:          "my ip",
			Name:              "TestActuator",
		},
	}

	mDaemon.On("Register", mock.Anything, mock.MatchedBy(func(r *protobufs.RegisterRequest) bool {
		return proto.Equal(r, &regReq)
	})).Return(nil, nil)

	resp, err := pluginManager.Register(ctx, &regReq)
	defer closer()
	assert.Nil(t, err)
	assert.True(t, resp.PluginRegistered)
}
