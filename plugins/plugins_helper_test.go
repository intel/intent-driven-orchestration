package pluginshelper

import (
	"syscall"
	"testing"
	"time"

	plugins "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
)

var (
	PluginManagerPort     = 33333
	Port                  = 33344
	Endpoint              = "some-endpoint"
	PluginManagerEndpoint = "some-service-ep"
	MongoEndpoint         = "mongodb://planner-mongodb-service:27017/"
)

func TestIsValidGenericConf(t *testing.T) {
	type genericConf struct {
		endpoint              string
		port                  int
		pluginManagerEndpoint string
		pluginManagerPort     int
		mongo                 string
	}
	tests := []struct {
		name    string
		args    genericConf
		wantErr bool
	}{
		{
			name:    "tc-0",
			args:    genericConf{Endpoint, Port, PluginManagerEndpoint, PluginManagerPort, "MongoEndpoint"},
			wantErr: true, // wrong url
		},
		{
			name:    "tc-1",
			args:    genericConf{Endpoint, Port, PluginManagerEndpoint, PluginManagerPort, ""},
			wantErr: true, // wrong url
		},
		{
			name:    "tc-2",
			args:    genericConf{Endpoint, Port, PluginManagerEndpoint, PluginManagerPort, MongoEndpoint},
			wantErr: false, // all good.
		},
		{
			name:    "tc-3",
			args:    genericConf{Endpoint, -10, PluginManagerEndpoint, PluginManagerPort, MongoEndpoint},
			wantErr: true, // negative port value
		},
		{
			name:    "tc-4",
			args:    genericConf{"", Port, PluginManagerEndpoint, PluginManagerPort, MongoEndpoint},
			wantErr: true, // wrong endpoint
		},
		{
			name:    "tc-5",
			args:    genericConf{Endpoint, 10000000, PluginManagerEndpoint, PluginManagerPort, MongoEndpoint},
			wantErr: true, // port to high.
		},
		{
			name:    "tc-6",
			args:    genericConf{Endpoint, Port, PluginManagerEndpoint, -20, MongoEndpoint},
			wantErr: true, // negative port value
		},
		{
			name:    "tc-7",
			args:    genericConf{Endpoint, Port, "", PluginManagerPort, MongoEndpoint},
			wantErr: true, // wrong endpoint
		},
		{
			name:    "tc-8",
			args:    genericConf{Endpoint, Port, PluginManagerEndpoint, 10000000, MongoEndpoint},
			wantErr: true, // port to high.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := IsValidGenericConf(tt.args.endpoint, tt.args.port, tt.args.pluginManagerEndpoint, tt.args.pluginManagerPort, tt.args.mongo); (err != nil) != tt.wantErr {
				t.Errorf("IsValidGenericConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsStrConfigValid(t *testing.T) {
	type args struct {
		str string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "tc-0", args: args{str: Endpoint}, want: true},
		{name: "tc-1", args: args{str: ""}, want: false},
		{name: "tc-2", args: args{str: "garbage inputs with very long line so that this should invalid given this is longer then one hundred and ten chars."}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStrConfigValid(tt.args.str); got != tt.want {
				t.Errorf("isStrValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPortNumValid(t *testing.T) {
	type args struct {
		num int
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "tc-1", args: args{num: 0}, want: false},
		{name: "tc-2", args: args{num: -10}, want: false},
		{name: "tc-3", args: args{num: 1000000}, want: false},
		{name: "tc-4", args: args{num: 33334}, want: true},
		{name: "tc-5", args: args{num: 0x100}, want: true},
		{name: "tc-6", args: args{num: 0x1000}, want: true},
		{name: "tc-7", args: args{num: 0x3fffffff}, want: false},
		{name: "tc-8", args: args{num: 0x7ffffffe}, want: false},
		{name: "tc-9", args: args{num: 0x7fffffff}, want: false},
		{name: "tc-10", args: args{num: 0x80000000}, want: false},
		{name: "tc-11", args: args{num: 0xfffffffe}, want: false},
		{name: "tc-12", args: args{num: 0xffffffff}, want: false},
		{name: "tc-13", args: args{num: 0x10000}, want: false},
		{name: "tc-14", args: args{num: 0x100000}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPortNumValid(tt.args.num); got != tt.want {
				t.Errorf("isNumValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

type DummyActuator struct {
}

func (d DummyActuator) Name() string {
	return "dummy"
}

func (d DummyActuator) Group() string {
	return "dummies"
}

func (d DummyActuator) NextState(_ *common.State, _ *common.State, _ map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	return nil, nil, nil
}

func (d DummyActuator) Perform(_ *common.State, _ []planner.Action) {

}

func (d DummyActuator) Effect(_ *common.State, _ map[string]common.Profile) {

}

func TestStartActuatorPluginForSuccess(t *testing.T) {
	var tmp []actuators.Actuator
	pluginManager := plugins.NewPluginManagerServer(tmp, "localhost", 33350)
	err := pluginManager.Start()
	if err != nil {
		t.Fatalf("Could not start plugin manager error was: %v", err)
	}

	actuator := DummyActuator{}
	exitChannel := StartActuatorPlugin(actuator, "localhost", 3350, "localhost", 33350)
	if err != nil {
		t.Errorf("Error should have been nil, was: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	exitChannel <- syscall.SIGINT

	err = pluginManager.Stop()
	if err != nil {
		t.Fatalf("Could not stop plugin manager error was: %v", err)
	}
}
