package pluginsHelper

import (
	"testing"
)

type genericConf struct {
	lookBack              int
	pluginManagerPort     int
	port                  int
	pythonInterpreter     string
	script                string
	endpoint              string
	pluginManagerEndpoint string
	mongo                 string
}

var (
	PluginManagerPort     = 33333
	Port                  = 33344
	LookBack              = 20
	Script                = "../pkg/planner/actuators/scaling/analytics/cpu_rightsizing.py"
	Endpoint              = "_XxIXS.10HMR1Nt0jaXi+ DKSvscN5312cB3TrQPEpSfEr/!|NXhZIZhEpeqNaxFNaxz9CMHo64iiCMgP9NfYVCiJzgRSFFFsxnb"
	PythonInterpreter     = "python3"
	PluginManagerEndpoint = "some-service-ep"
	MongoEndpoint         = "mongodb://planner-mongodb-service:27017/"
)

func setConfigValues(lookBack, port, pluginManagerPort int, pythonInterpreter, script, endpoint, pluginManagerEndpoint, mongo string) genericConf {
	return genericConf{
		lookBack:              lookBack,
		pluginManagerPort:     pluginManagerPort,
		port:                  port,
		pythonInterpreter:     pythonInterpreter,
		script:                script,
		endpoint:              endpoint,
		pluginManagerEndpoint: pluginManagerEndpoint,
		mongo:                 mongo,
	}
}

func TestIsValidCPUGenericConf(t *testing.T) {
	tests := []struct {
		name    string
		args    genericConf
		wantErr bool
	}{
		{
			name: "tc",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: false, // negative value for time
		},
		{
			name: "tc-1",
			args: setConfigValues(-20, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // negative value for time
		},
		{
			name: "tc-2",
			args: setConfigValues(0, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // zero value for time
		},
		{
			name: "tc-3",
			args: setConfigValues(2222222222222220, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // over limit time value
		},
		{
			name: "tc-4",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, "script",
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // wrong Script path
		},
		{
			name: "tc-5",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, "",
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // wrong Script path
		},
		{
			name: "tc-6",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, "MongoEndpoint"),
			wantErr: true, // wrong url
		},
		{
			name: "tc-7",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, ""),
			wantErr: true, // wrong url
		},
		{
			name: "tc-8",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, "someUrl.co:23454"),
			wantErr: false, // wrong url
		},
		{
			name: "tc-9",
			args: setConfigValues(20, -1, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // wrong url
		},
		{
			name: "tc-9",
			args: setConfigValues(20, -1, PluginManagerPort, PythonInterpreter, Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // negative port value
		},
		{
			name: "tc-10",
			args: setConfigValues(20, Port, PluginManagerPort, PythonInterpreter, Script,
				"", PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // no endpoint defined
		},
		{
			name: "tc-11",
			args: setConfigValues(20, Port, PluginManagerPort, "", Script,
				Endpoint, PluginManagerEndpoint, MongoEndpoint),
			wantErr: true, // no python
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := IsValidGenericConf(tt.args.lookBack, tt.args.pluginManagerPort, tt.args.port,
				tt.args.pythonInterpreter, tt.args.script, tt.args.endpoint, tt.args.pluginManagerEndpoint,
				tt.args.mongo); (err != nil) != tt.wantErr {
				t.Errorf("IsValidCPUGenericConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_isStrValid(t *testing.T) {
	type args struct {
		str string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "tc-1", args: args{str: ""}, want: false},
		{name: "tc-2", args: args{str: Endpoint}, want: true},
		{name: "tc-3", args: args{str: Endpoint + "1"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStrValid(tt.args.str); got != tt.want {
				t.Errorf("isStrValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isPortNumValid(t *testing.T) {
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
