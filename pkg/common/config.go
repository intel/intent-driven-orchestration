package common

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	goRuntime "runtime"

	"k8s.io/klog/v2"
)

// Config holds all the configuration information.
type Config struct {
	Generic    GenericConfig    `json:"generic"`
	Controller ControllerConfig `json:"controller"`
	Monitor    MonitorConfig    `json:"monitor"`
	Planner    PlannerConfig    `json:"planner"`
}

// GenericConfig captures generic configuration fields.
type GenericConfig struct {
	MongoEndpoint string `json:"mongo_endpoint"`
}

// ControllerConfig holds controller related configs.
type ControllerConfig struct {
	Workers           int    `json:"workers"`
	TaskChannelLength int    `json:"task_channel_length"`
	InformerTimeout   int    `json:"informer_timeout"`
	ControllerTimeout int    `json:"controller_timeout"`
	PlanCacheTTL      int    `json:"plan_cache_ttl"`
	PlanCacheTimeout  int    `json:"plan_cache_timeout"`
	TelemetryEndpoint string `json:"telemetry_endpoint"`
	HostField         string `json:"host_field"`
	Metrics           []struct {
		Name  string `json:"name,omitempty"`
		Query string `json:"query,omitempty"`
	} `json:"metrics"`
}

// MonitorConfig holds monitor related configs.
type MonitorConfig struct {
	Pod struct {
		Workers int `json:"workers"`
	} `json:"pod"`
	Profile struct {
		Workers int    `json:"workers"`
		Queries string `json:"queries"`
	} `json:"profile"`
	Intent struct {
		Workers int `json:"workers"`
	} `json:"intent"`
}

// PlannerConfig holds planner related configs.
// TODO: fuzz test max states, candidates etc.
type PlannerConfig struct {
	AStar struct {
		OpportunisticCandidates int    `json:"opportunistic_candidates"`
		MaxStates               int    `json:"max_states"`
		MaxCandidates           int    `json:"max_candidates"`
		PluginManagerEndpoint   string `json:"plugin_manager_endpoint"`
		PluginManagerPort       int    `json:"plugin_manager_port"`
	} `json:"astar"`
}

const (
	// MaxControllerTimeout is max timeout (s) between each intent's reevaluation.
	MaxControllerTimeout = 600
	// MaxInformerTimeout is max timeout (s) for the informer factories for the CRDs and PODs.
	MaxInformerTimeout = 300
	// MaxTaskChannelLen is max length for job queue for processing intents.
	MaxTaskChannelLen = 10000
	// MaxPlanCacheTimeout is max timeout (ms) between each intent's reevaluation.
	MaxPlanCacheTimeout = 50000
	// MaxPlanCacheTTL is max time-to-live (ms) for an entry in the planner's cache.
	MaxPlanCacheTTL = 500000
)

// maximumWorkers maximum number of logical cores for workers.
var maximumWorkers = goRuntime.NumCPU()

// LoadConfig reads the configuration file and marshals it into an object.
func LoadConfig(filename string, createType func() interface{}) (interface{}, error) {
	tmp, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to read config file: %s", err)
	}
	cfg := createType()
	err = json.Unmarshal(tmp, cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to parse config file: %s", err)
	}
	return cfg, nil
}

// ParseConfig loads the configuration from a JSON file.
func ParseConfig(filename string) (Config, error) {
	tmp, err := LoadConfig(filename, func() interface{} {
		return &Config{}
	})
	if err != nil {
		return Config{}, fmt.Errorf("error parsing config: %s", err)
	}
	result := tmp.(*Config)

	if result.Controller.TaskChannelLength <= 0 ||
		result.Controller.TaskChannelLength > MaxTaskChannelLen ||
		result.Controller.ControllerTimeout <= 0 ||
		result.Controller.ControllerTimeout > MaxControllerTimeout ||
		result.Controller.InformerTimeout <= 0 ||
		result.Controller.InformerTimeout > MaxInformerTimeout ||
		result.Controller.PlanCacheTimeout <= 0 ||
		result.Controller.PlanCacheTimeout > MaxPlanCacheTimeout ||
		result.Controller.PlanCacheTTL <= 0 ||
		result.Controller.PlanCacheTTL > MaxPlanCacheTTL {
		return *result, fmt.Errorf("invalid input value: Out of the provided limits")
	}
	if invalidWorkers(result.Controller.Workers) ||
		invalidWorkers(result.Monitor.Profile.Workers) ||
		invalidWorkers(result.Monitor.Intent.Workers) {
		return *result, fmt.Errorf("invalid worker(s) number")
	}
	if result.Planner.AStar.OpportunisticCandidates < 0 ||
		result.Planner.AStar.OpportunisticCandidates > 1000 {
		return *result, fmt.Errorf("invalid input value: Out of the provided limits")
	}
	if result.Planner.AStar.PluginManagerPort < 1 ||
		result.Planner.AStar.PluginManagerPort > 65535 {
		return *result, fmt.Errorf("invalid input value: Port number is not in a valid range: %d", result.Planner.AStar.PluginManagerPort)
	}
	if !checkURL(result.Controller.TelemetryEndpoint) || !checkURL(result.Generic.MongoEndpoint) {
		return *result, fmt.Errorf("invalid URL")
	}

	return *result, nil
}

// invalidWorkers checks if the numbers of workers is within a valid range.
func invalidWorkers(nWorkers int) bool {
	if nWorkers <= 0 || nWorkers > maximumWorkers {
		return true
	}
	return false
}

// checkURL validate if the input url is fine.
func checkURL(urlpath string) bool {
	_, err := url.ParseRequestURI(urlpath)
	if err != nil {
		klog.Error(err)
		return false
	}
	return true
}
