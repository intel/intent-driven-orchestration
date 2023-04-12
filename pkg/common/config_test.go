package common

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"k8s.io/klog/v2"
)

// Tests for success.

// TestPodAvailabilityForSuccess tests for success.
func TestParseConfigForSuccess(t *testing.T) {
	_, err := ParseConfig("../../defaults.json")
	if err != nil {
		t.Errorf("Now this should have worked :-)")
	}
}

// Tests for failure.

// TestParseConfigForFailure tests for failure.
func TestParseConfigForFailure(t *testing.T) {
	_, err := ParseConfig("foo.yaml")
	if err == nil {
		t.Errorf("The code did not return an error!")
	}

	_, err = ParseConfig("config.go")
	if err == nil {
		t.Errorf("The code did not return an error!")
	}
}

// Tests for sanity.

// n/a

func TestCheckURL(t *testing.T) {
	type args struct {
		urlpath string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"tc-1", args{urlpath: "mongodb://planner-mongodb-service:27017/"}, false},
		{"tc-2", args{urlpath: "xjkldaoiu/"}, true},
		{"tc-3", args{urlpath: "http://prometheus-operated.monitoring:9090/api/v1/query"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkURL(tt.args.urlpath); got == tt.wantErr {
				t.Errorf("checkURL() = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	type args struct {
		filename string
	}
	configUnmashal := SetupTestConfigFile(2, 100, 30, 45, 45000, 5000,
		2, 2, 2,
		0, 2000, 10, 33333,
		"mongodb://planner-mongodb-service:27017/",
		"http://prometheus-service.telemetry:9090/api/v1/query",
		"exported_instance",
		"cpu_value",
		"avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)",
		"artefacts/examples/default_queries.json",
		"plugin-manager-service")
	configMashal, err := json.MarshalIndent(configUnmashal, "", " ")
	if err != nil {
		klog.Error(err)
		return
	}
	filename := "test-case1.json"
	err = os.WriteFile(filename, configMashal, 0600)
	if err != nil {
		klog.Error(err)
		return
	}
	configUnmashal = SetupTestConfigFile(2, 100, 30, 45, 45000, 5000,
		2, 2, 2,
		0, 2000, 10, -33333,
		"mongodb://planner-mongodb-service:27017/",
		"http://prometheus-service.telemetry:9090/api/v1/query",
		"exported_instance",
		"cpu_value",
		"avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)",
		"artefacts/examples/default_queries.json",
		"plugin-manager-service")
	configMashal, err = json.MarshalIndent(configUnmashal, "", " ")
	if err != nil {
		klog.Error(err)
		return
	}
	filename = "test-case2.json"
	err = os.WriteFile(filename, configMashal, 0600)
	if err != nil {
		klog.Error(err)
		return
	}

	tests := []struct {
		name    string
		args    args
		want    Config
		wantErr bool
	}{
		{
			name: "tc-1",
			args: args{filename: "test-case1.json"},
			want: Config{
				Generic: GenericConfig{MongoEndpoint: "mongodb://planner-mongodb-service:27017/"},
				Controller: ControllerConfig{
					Workers:           2,
					TaskChannelLength: 100,
					InformerTimeout:   30,
					ControllerTimeout: 45,
					PlanCacheTTL:      45000,
					PlanCacheTimeout:  5000,
					TelemetryEndpoint: "http://prometheus-service.telemetry:9090/api/v1/query",
					HostField:         "exported_instance",
					Metrics: []struct {
						Name  string "json:\"name,omitempty\""
						Query string "json:\"query,omitempty\""
					}{
						{Name: "cpu_value", Query: "avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)"},
					},
				},
				Monitor: MonitorConfig{
					Pod: struct {
						Workers int "json:\"workers\""
					}{
						Workers: 2,
					},
					Profile: struct {
						Workers int    "json:\"workers\""
						Queries string "json:\"queries\""
					}{
						Workers: 2,
						Queries: "artefacts/examples/default_queries.json",
					},
					Intent: struct {
						Workers int "json:\"workers\""
					}{
						Workers: 2,
					},
				},
				Planner: PlannerConfig{
					AStar: struct {
						OpportunisticCandidates int    "json:\"opportunistic_candidates\""
						MaxStates               int    "json:\"max_states\""
						MaxCandidates           int    "json:\"max_candidates\""
						PluginManagerEndpoint   string "json:\"plugin_manager_endpoint\""
						PluginManagerPort       int    "json:\"plugin_manager_port\""
					}{
						OpportunisticCandidates: 0,
						MaxStates:               2000,
						MaxCandidates:           10,
						PluginManagerEndpoint:   "plugin-manager-service",
						PluginManagerPort:       33333,
					},
				},
			},
			wantErr: false, // no errors
		},
		{
			name: "tc-2",
			args: args{filename: "test-case2.json"},
			want: Config{
				Generic: GenericConfig{MongoEndpoint: "mongodb://planner-mongodb-service:27017/"},
				Controller: ControllerConfig{
					Workers:           2,
					TaskChannelLength: 100,
					InformerTimeout:   30,
					ControllerTimeout: 45,
					PlanCacheTTL:      45000,
					PlanCacheTimeout:  5000,
					TelemetryEndpoint: "http://prometheus-service.telemetry:9090/api/v1/query",
					HostField:         "exported_instance",
					Metrics: []struct {
						Name  string "json:\"name,omitempty\""
						Query string "json:\"query,omitempty\""
					}{
						{Name: "cpu_value", Query: "avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)"},
					},
				},
				Monitor: MonitorConfig{
					Pod: struct {
						Workers int "json:\"workers\""
					}{
						Workers: 2,
					},
					Profile: struct {
						Workers int    "json:\"workers\""
						Queries string "json:\"queries\""
					}{
						Workers: 2,
						Queries: "artefacts/examples/default_queries.json",
					},
					Intent: struct {
						Workers int "json:\"workers\""
					}{
						Workers: 2,
					},
				},
				Planner: PlannerConfig{
					AStar: struct {
						OpportunisticCandidates int    "json:\"opportunistic_candidates\""
						MaxStates               int    "json:\"max_states\""
						MaxCandidates           int    "json:\"max_candidates\""
						PluginManagerEndpoint   string "json:\"plugin_manager_endpoint\""
						PluginManagerPort       int    "json:\"plugin_manager_port\""
					}{
						OpportunisticCandidates: 0,
						MaxStates:               2000,
						MaxCandidates:           10,
						PluginManagerEndpoint:   "plugin-manager-service",
						PluginManagerPort:       -33333, // invalid port
					},
				},
			},
			wantErr: true, // want error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errParse := ParseConfig(tt.args.filename)
			if errParse != nil {
				klog.Error(errParse)
			}
			err = os.RemoveAll(tt.args.filename)
			if err != nil {
				klog.Error(err)
			}
			if (errParse != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", errParse, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func SetupTestConfigFile(
	workers,
	taskchannellength,
	informerTimeout,
	controllerTimeout,
	planCacheTTL,
	planCacheTimeout int,
	podWorker,
	profileWorker,
	intentWorker int,
	opportunisticCandidates,
	maxStates,
	maxCandidates,
	pluginManagerPort int,
	mongoEndpoint,
	telemetryEndpoint,
	hostField,
	metricName,
	metricQuery,
	profileQuery,
	pluginEndpoint string) Config {
	return Config{
		Generic: GenericConfig{MongoEndpoint: mongoEndpoint},
		Controller: ControllerConfig{
			Workers:           workers,
			TaskChannelLength: taskchannellength,
			InformerTimeout:   informerTimeout,
			ControllerTimeout: controllerTimeout,
			PlanCacheTTL:      planCacheTTL,
			PlanCacheTimeout:  planCacheTimeout,
			TelemetryEndpoint: telemetryEndpoint,
			HostField:         hostField,
			Metrics: []struct {
				Name  string "json:\"name,omitempty\""
				Query string "json:\"query,omitempty\""
			}{
				{Name: metricName, Query: metricQuery},
			},
		},
		Monitor: MonitorConfig{
			Pod: struct {
				Workers int "json:\"workers\""
			}{
				Workers: podWorker,
			},
			Profile: struct {
				Workers int    "json:\"workers\""
				Queries string "json:\"queries\""
			}{
				Workers: profileWorker,
				Queries: profileQuery,
			},
			Intent: struct {
				Workers int "json:\"workers\""
			}{
				Workers: intentWorker,
			},
		},
		Planner: PlannerConfig{
			AStar: struct {
				OpportunisticCandidates int    "json:\"opportunistic_candidates\""
				MaxStates               int    "json:\"max_states\""
				MaxCandidates           int    "json:\"max_candidates\""
				PluginManagerEndpoint   string "json:\"plugin_manager_endpoint\""
				PluginManagerPort       int    "json:\"plugin_manager_port\""
			}{
				OpportunisticCandidates: opportunisticCandidates,
				MaxStates:               maxStates,
				MaxCandidates:           maxCandidates,
				PluginManagerEndpoint:   pluginEndpoint,
				PluginManagerPort:       pluginManagerPort,
			},
		},
	}
}
