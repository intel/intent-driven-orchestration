package energy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"k8s.io/utils/strings/slices"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// powerActionName represents the name for this action.
const powerActionName = "setPowerProfile"

// groupName represents the name for this group.
const groupName = "energy"

// PowerActuatorConfig describes the configuration for this actuator.
type PowerActuatorConfig struct {
	PythonInterpreter     string   `json:"interpreter"`
	PowerProfiles         []string `json:"options"`
	Prediction            string   `json:"prediction"`
	Analytics             string   `json:"analytics"`
	ProactiveCandidates   bool     `json:"add_proactive_candidates"`
	RenewableLimit        float64  `json:"renewable_limit"`
	StepDown              int      `json:"step_down"`
	Endpoint              string   `json:"endpoint"`
	Port                  int      `json:"port"`
	PluginManagerEndpoint string   `json:"plugin_manager_endpoint"`
	PluginManagerPort     int      `json:"plugin_manager_port"`
	MongoEndpoint         string   `json:"mongo_endpoint"`
}

// PowerActuator is an actuator that can handle power management related settings.
type PowerActuator struct {
	config PowerActuatorConfig
	client kubernetes.Interface
	Cmd    *exec.Cmd
}

// requestBody represents the json send to prediction function.
type requestBody struct {
	Intent     string   `json:"intent"`
	Objectives []string `json:"objectives"`
	Cores      int      `json:"cores"`
}

// doQuery calls the prediction function.
func doQuery(body requestBody) map[string][]float64 {
	tmp, _ := json.Marshal(body)
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Post("http://127.0.0.1:8321", "application/json", bytes.NewBuffer(tmp))
	if err != nil {
		klog.Errorf("Could not reach prediction endpoint: %s.", err)
		return nil
	}
	if resp.StatusCode != 200 {
		klog.Errorf("Received non 200 status code: %d - %s.", resp.StatusCode, resp.Status)
		return nil
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			klog.Errorf("Error while handling body: %s.", err)
		}
	}(resp.Body)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("Error to read the body: %s", err)
	}
	var res map[string][]float64
	err = json.Unmarshal(respBody, &res)
	if err != nil {
		klog.Errorf("Could not unmarshall response: %s - %v.", err, body)
		return nil
	}
	return res
}

func (power PowerActuator) Name() string {
	return powerActionName
}

func (power PowerActuator) Group() string {
	return groupName
}

// contains figures out if a value is part of a slice.
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if value == v {
			return true
		}
	}
	return false
}

// findProfile uses forecasted values to determine the best profile or return a set of possible options.
func (power PowerActuator) findProfile(intentKey string, objectives []string, targets map[string]float64, cpuAsk int64, onlyPower bool) map[string]map[string]float64 {
	tmp := doQuery(requestBody{Intent: intentKey, Objectives: objectives, Cores: int(cpuAsk / 1000.0)})
	if tmp == nil {
		return map[string]map[string]float64{}
	}
	candidate := map[string]float64{}
	profileName := "None"
	//// run through all profiles...
	for i, profile := range power.config.PowerProfiles {
		// ...and if it fits all the required targets let's send back what we found.
		found := false
		for _, objective := range objectives {
			forecast, ok := tmp[objective]
			if ok {
				if forecast[i] <= targets[objective] {
					found = true
				} else {
					found = false
					break
				}
			} else {
				klog.Warningf("Forecasted value for objective %s not found for %s; maybe no model available?", objective, intentKey)
				return map[string]map[string]float64{}
			}
		}
		if found {
			profileName = profile
			for _, objective := range objectives {
				candidate[objective] = tmp[objective][i]
			}
			if !onlyPower {
				return map[string]map[string]float64{profile: candidate}
			}
		}
	}
	if onlyPower {
		return map[string]map[string]float64{profileName: candidate}
	}
	if power.config.ProactiveCandidates {
		klog.V(1).Infof("Proactive mode enabled - will return all possible options.")
		options := map[string]map[string]float64{}
		for i, profile := range power.config.PowerProfiles {
			forecast := map[string]float64{}
			for _, objective := range objectives {
				forecast[objective] = tmp[objective][i]
			}
			options[profile] = forecast
		}
		return options
	}
	klog.Warningf("Could not find a profile that fits the bill for %s: %v-%v.", intentKey, objectives, targets)
	return map[string]map[string]float64{}
}

func (power PowerActuator) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	klog.V(2).Infof("NextState called.")
	_, ok := state.CurrentData[power.Name()]
	if ok {
		// we already did a power optimization - we are done...
		return nil, nil, nil
	}

	// Check what the current profile is, if any...
	// If no resource requests are set, or
	// this is not in a guaranteed class, or
	// we've not PODs atm...return.
	if len(state.CurrentPods) < 1 {
		klog.V(2).Infof("Nothing todo - as workload has not PODs.")
		return nil, nil, nil
	}

	// make sure we only operate on guaranteed PODs.
	for _, podState := range state.CurrentPods {
		if podState.QoSClass != "Guaranteed" {
			klog.Warningf("Targeted workload is not in guaranteed QoS class: %s.", state.Intent.Key)
			return nil, nil, nil
		}
		break // all PODs are equal.
	}

	// find current profile - if any.
	currentProfile := "None"
	cpuAsk := int64(-1)
	cpuAllocated := int64(-1)
	lastCont := -1
	for resourceKey, value := range state.Resources {
		tmp := strings.Split(resourceKey, "_")
		if len(tmp) != 3 {
			continue
		}
		currentContainerID, err := strconv.Atoi(tmp[0])
		if err != nil {
			return nil, nil, nil
		}
		if currentContainerID >= lastCont {
			lastCont = currentContainerID
			if tmp[1] == "cpu" && tmp[2] == "requests" {
				cpuAsk = value
			}
			for _, profile := range power.config.PowerProfiles {
				if strings.Contains(tmp[1], profile) && tmp[2] == "requests" {
					currentProfile = profile
					cpuAllocated = value
					break
				}
			}
		}
	}

	// Determine what we know think is the best power profile.
	var objectiveNames []string
	onlyPower := true
	for key := range goal.Intent.Objectives {
		if profiles[key].ProfileType != common.ProfileTypeFromText("latency") && profiles[key].ProfileType != common.ProfileTypeFromText("power") {
			continue
		}
		if profiles[key].ProfileType == common.ProfileTypeFromText("latency") {
			onlyPower = false
		}
		objectiveNames = append(objectiveNames, key)
	}
	forecast := power.findProfile(state.Intent.Key, objectiveNames, goal.Intent.Objectives, cpuAsk, onlyPower)

	// get current RER.
	rer := 100.0
	for _, item := range state.CurrentData["renewable_energy_ratio"] {
		rer = item
		break
	}

	if len(forecast) == 1 {
		var newProfile string
		var values map[string]float64
		for k, v := range forecast {
			newProfile = k
			values = v
			break
		}
		klog.V(1).Infof("Best new power profile is: %s - current: %s", newProfile, currentProfile)

		// If we've got a valid profile, and it is not the same as current - return action...
		// If cpu allocation changed - the power profile needs to be updated too...
		// and our renewable energy ratio is above the limit...
		if ((contains(power.config.PowerProfiles, newProfile) && currentProfile != newProfile) ||
			(currentProfile == newProfile && cpuAsk != cpuAllocated && cpuAllocated != -1)) && rer >= power.config.RenewableLimit {
			utility := 0.0
			newState := state.DeepCopy()
			newState.CurrentData[power.Name()] = map[string]float64{powerActionName: 1.0}
			for _, key := range objectiveNames {
				newState.Intent.Objectives[key] = values[key]
			}
			action := planner.Action{Name: power.Name(), Properties: map[string]string{"profile": newProfile}}
			return []common.State{newState}, []float64{utility}, []planner.Action{action}
		} else if rer < power.config.RenewableLimit && currentProfile != "None" {
			utility := state.Intent.Priority
			newState := state.DeepCopy()
			newState.CurrentData[power.Name()] = map[string]float64{powerActionName: 1.0}
			// We are setting the objectives to the goal state so the planner favors this action.
			for _, key := range objectiveNames {
				newState.Intent.Objectives[key] = goal.Intent.Objectives[key]
			}
			idx := slices.Index(power.config.PowerProfiles, newProfile)
			if idx-power.config.StepDown >= 1 {
				idx -= power.config.StepDown
			} else {
				// Take first entry.
				idx = 0
			}
			if power.config.PowerProfiles[idx] != currentProfile {
				action := planner.Action{Name: power.Name(), Properties: map[string]string{"profile": power.config.PowerProfiles[idx]}}
				return []common.State{newState}, []float64{utility}, []planner.Action{action}
			}
		}
	} else if len(forecast) > 1 && rer >= power.config.RenewableLimit {
		// proactive case - we can give the planner some options now.
		klog.V(1).Infof("Could not find best option, but proactive mode is enabled, so offering candidates: %v.", forecast)
		var states []common.State
		var utils []float64
		var actions []planner.Action
		for profile, values := range forecast {
			utility := 0.0
			newState := state.DeepCopy()
			newState.CurrentData[power.Name()] = map[string]float64{powerActionName: 1.0}
			for _, key := range objectiveNames {
				newState.Intent.Objectives[key] = values[key]
			}
			action := planner.Action{Name: power.Name(), Properties: map[string]string{"profile": profile}}
			states = append(states, newState)
			utils = append(utils, utility)
			actions = append(actions, action)
		}
		return states, utils, actions
	}
	return nil, nil, nil
}

func (power PowerActuator) Perform(state *common.State, plan []planner.Action) {
	klog.V(2).Infof("Perform called.")
	var profile string
	found := false
	for _, item := range plan {
		if item.Name == power.Name() {
			profile = item.Properties.(map[string]string)["profile"]
			found = true
			break
		}
	}
	if !found {
		return
	}
	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]
	name := tmp[1]

	if state.Intent.TargetKind == "Deployment" {
		depl, err := power.client.AppsV1().Deployments(namespace).Get(context.TODO(), name, metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("Failed to retrieve deployment: %v", err)
			return
		}
		newDepl := depl.DeepCopy()
		for _, container := range newDepl.Spec.Template.Spec.Containers {
			resourceRequests := container.Resources.Requests
			resourceLimits := container.Resources.Limits

			// remove any old power profile setting...
			for _, candidate := range power.config.PowerProfiles {
				delete(resourceRequests, coreV1.ResourceName(candidate))
				delete(resourceLimits, coreV1.ResourceName(candidate))
			}
			if profile != "None" { // This way we can also move the POD back to the shared pool...
				resourceRequests[coreV1.ResourceName(profile)] = *resourceRequests.Cpu()
				resourceLimits[coreV1.ResourceName(profile)] = *resourceRequests.Cpu()
			}

			container.Resources.Requests = resourceRequests
			container.Resources.Limits = resourceLimits
		}
		_, err = power.client.AppsV1().Deployments(namespace).Update(context.TODO(), newDepl, metaV1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Failed to update deployment: %v", err)
		}
	} else {
		klog.Warningf("Can only update Deployment for now! Your type was: %s.", state.Intent.TargetKind)
	}
}

func (power PowerActuator) Effect(state *common.State, profiles map[string]common.Profile) {
	klog.V(2).Infof("Effect called.")
	var names []string
	for name := range state.Intent.Objectives {
		if profiles[name].ProfileType != common.ProfileTypeFromText("latency") && profiles[name].ProfileType != common.ProfileTypeFromText("power") {
			continue
		}
		names = append(names, name)
	}
	cmd := exec.Command(power.config.PythonInterpreter, power.config.Analytics, state.Intent.Key, strings.Join(names, ","), strings.Join(power.config.PowerProfiles, ",")) //#nosec G204 -- NA
	out, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("Error triggering analytics script: %s - %s.", err, string(out))
	}
}

// NewPowerActuator initializes a new actuator.
func NewPowerActuator(core kubernetes.Interface, _ controller.Tracer, cfg PowerActuatorConfig) *PowerActuator {
	cmd := exec.Command(cfg.PythonInterpreter, cfg.Prediction, strings.Join(cfg.PowerProfiles, ",")) //#nosec G204 -- NA
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		klog.Fatalf("Could not start the prediction web service: %s.", err)
	}
	time.Sleep(500 * time.Millisecond)

	return &PowerActuator{
		config: cfg,
		client: core,
		Cmd:    cmd,
	}
}
