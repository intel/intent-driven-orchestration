package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	"golang.org/x/exp/maps"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// TODO: using python scripts is fine for demo purposes, but we need to replace them; instantiation is slow.

// rdtActionName represents the name action to configure a POD's RDT related configs.
const rdtActionName = "config_rdt"

// rdtGroupName represents the name for all actions related to Intel RDT.
const rdtGroupName = "rdt"

// RdtConfig holds the specific configs for this actuator.
type RdtConfig struct {
	Interpreter           string   `json:"interpreter"`
	Analytics             string   `json:"analytics_script"`
	Prediction            string   `json:"prediction_script"`
	Options               []string `json:"options"`
	Port                  int      `json:"port"`
	Endpoint              string   `json:"endpoint"`
	MongoEndpoint         string   `json:"mongo_endpoint"`
	PluginManagerEndpoint string   `json:"plugin_manager_endpoint"`
	PluginManagerPort     int      `json:"plugin_manager_port"`
}

// RdtActuator represents the actual RDT actuator.
type RdtActuator struct {
	config RdtConfig
	tracer controller.Tracer
	k8s    kubernetes.Interface
}

func (rdt RdtActuator) Name() string {
	return rdtActionName
}

func (rdt RdtActuator) Group() string {
	return rdtGroupName
}

// requestBody represents the json send to prediction function.
type requestBody struct {
	Name     string  `json:"name"`
	Target   string  `json:"target"`
	Option   string  `json:"option"`
	Load     float64 `json:"load"`
	LLCValue float64 `json:"llc_value"`
	Class    string  `json:"class"`
	Replicas int     `json:"replicas"`
}

// responseBody represents the json returned by the prediction function.
type responseBody struct {
	Val float64 `json:"val"`
}

// doQuery calls the prediction function.
func doQuery(body requestBody) float64 {
	tmp, _ := json.Marshal(body)
	resp, err := http.Post("http://localhost:8000", "application/json", bytes.NewBuffer(tmp))
	if err != nil {
		klog.Errorf("Could not reach prediction endpoint: %s.", err)
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
	var res responseBody
	err = json.Unmarshal(respBody, &res)
	if err != nil {
		klog.Errorf("Could not unmarshall response: %s - %v.", err, body)
		return -1.0
	}
	return res.Val
}

func (rdt RdtActuator) findFollowUpState(start *common.State, goal *common.State, profiles map[string]common.Profile) (common.State, string, float64) {
	distance := start.Distance(goal, profiles)
	var res common.State
	var selectedOption string
	var utility float64
	for _, option := range rdt.config.Options {
		newState := start.DeepCopy()
		tempSum := 0.0
		tempPredSum := 0.0
		found := true

		// predict effect of this config option...
		for k := range newState.Intent.Objectives {
			if profiles[k].ProfileType != common.ProfileTypeFromText("latency") {
				// only work on latency related targets.
				continue
			}

			// required parameters.
			load := 0.0
			for _, val := range newState.CurrentData["cpu_value"] {
				load += val
			}
			if len(newState.CurrentData["cpu_value"]) > 1 {
				load /= float64(len(newState.CurrentData["cpu_value"]))
			}
			llcLoad := 0.0
			for _, val := range newState.CurrentData["llc_value"] {
				llcLoad += val
			}
			if len(newState.CurrentData["llc_value"]) > 1 {
				llcLoad /= float64(len(newState.CurrentData["llc_value"]))
			}
			qosClass := "Burstable" // TODO: pick up from pod specs.
			replicas := len(newState.CurrentPods)

			// actually predict.
			body := requestBody{
				Name:     newState.Intent.Key,
				Target:   k,
				Option:   option,
				Load:     load,
				LLCValue: llcLoad,
				Class:    qosClass,
				Replicas: replicas,
			}
			predictedValue := doQuery(body)
			if predictedValue == -1.0 {
				// Predict script couldn't figure sth out -> need to skip this option.
				found = false
				break
			}

			newState.Intent.Objectives[k] = predictedValue
			tempPredSum += predictedValue
			tempSum += goal.Intent.Objectives[k]
		}

		// test if better and closer...
		if newState.Compare(goal, profiles) && newState.Distance(goal, profiles) < distance && found {
			for _, pod := range newState.CurrentPods {
				if len(pod.Annotations) > 0 {
					pod.Annotations["rdt_visited"] = ""
				} else {
					pod.Annotations = map[string]string{"rdt_visited": ""}
				}
			}
			selectedOption = option
			if tempPredSum < tempSum {
				utility = tempPredSum / tempSum * goal.Intent.Priority
			} else {
				utility = tempPredSum / tempSum * 1 / goal.Intent.Priority
			}
			distance = newState.Distance(goal, profiles)
			res = newState
		}
	}
	return res, selectedOption, utility
}

func (rdt RdtActuator) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	klog.V(2).Infof("Finding next state for %s.", state.Intent.Key)
	currentOption := "None"
	// we do not support recursive action calls in the state graph.
	if len(state.CurrentPods) > 0 {
		podKeys := maps.Keys(state.CurrentPods)
		aPod := state.CurrentPods[podKeys[0]]
		if _, found := aPod.Annotations["rdt_config"]; found {
			currentOption = aPod.Annotations["rdt_config"]
		}
		if _, found := aPod.Annotations["rdt_visited"]; found {
			return nil, nil, nil
		}
	}

	// find a good follow-up state...
	newState, option, utility := rdt.findFollowUpState(state, goal, profiles)
	if len(option) > 0 && option != currentOption {
		action := planner.Action{Name: rdt.Name(), Properties: map[string]string{"option": option}}
		return []common.State{newState}, []float64{utility}, []planner.Action{action}
	}
	return nil, nil, nil
}

func (rdt RdtActuator) Perform(state *common.State, plan []planner.Action) {
	klog.V(2).Infof("%s-%s - performing plan: %v", rdt.Group(), rdt.Name(), plan)
	var option string
	for _, item := range plan {
		if item.Name == rdt.Name() {
			option = item.Properties.(map[string]string)["option"]
		}
	}
	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]

	// set annotation
	for podName := range state.CurrentPods {
		res, err := rdt.k8s.CoreV1().Pods(namespace).Get(context.TODO(), podName, metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("failed to get version of POD: %v", err)
			return
		}
		newPod := res.DeepCopy()
		annotations := newPod.ObjectMeta.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}
		if option != "None" {
			annotations["rdt_config"] = option
		} else {
			delete(annotations, "rdt_config")
		}
		newPod.ObjectMeta.Annotations = annotations
		_, err = rdt.k8s.CoreV1().Pods(res.ObjectMeta.Namespace).Update(context.TODO(), newPod, metaV1.UpdateOptions{})
		if err != nil {
			klog.Errorf("failed to get update POD: %v", err)
			return
		}
	}
	// TODO: add NFD/NPD requirement label.
	// TODO: set any hints for scheduler.
}

func (rdt RdtActuator) Effect(state *common.State, profiles map[string]common.Profile) {
	klog.V(2).Infof("Triggering effect calculation for: %v", state.Intent.Objectives)
	for name := range state.Intent.Objectives {
		if profiles[name].ProfileType != common.ProfileTypeFromText("latency") {
			continue
		}
		cmd := exec.Command(rdt.config.Interpreter, rdt.config.Analytics, state.Intent.Key, name) //#nosec G204 -- NA
		out, err := cmd.CombinedOutput()
		if err != nil {
			klog.Errorf("Error triggering analytics script: %s - %s.", err, string(out))
		}
	}
}

// NewRdtActuator initializes a new actuator.
func NewRdtActuator(client kubernetes.Interface, tracer controller.Tracer, cfg RdtConfig) *RdtActuator {
	cmd := exec.Command(cfg.Interpreter, cfg.Prediction) //#nosec G204 -- NA
	err := cmd.Start()
	if err != nil {
		klog.Errorf("Could not start the prediction script: %s.", err)
	}
	time.Sleep(500 * time.Millisecond)

	return &RdtActuator{
		config: cfg,
		tracer: tracer,
		k8s:    client,
	}
}
