package platform

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

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// TODO: using python scripts is fine for demo purposes, but we need to replace them; instantiation is slow.

// rdtActionName represents the name action to configure a POD's RDT related configs.
const rdtActionName = "configureRDT"

// rdtGroupName represents the name for all actions related to Intel RDT.
const rdtGroupName = "rdt"

// RdtConfig holds the specific configs for this actuator.
type RdtConfig struct {
	Interpreter           string   `json:"interpreter"`
	Analytics             string   `json:"analytics_script"`
	Prediction            string   `json:"prediction_script"`
	Options               []string `json:"options"`
	AnnotationName        string   `json:"annotation_name"`
	Endpoint              string   `json:"endpoint"`
	Port                  int      `json:"port"`
	PluginManagerEndpoint string   `json:"plugin_manager_endpoint"`
	PluginManagerPort     int      `json:"plugin_manager_port"`
	MongoEndpoint         string   `json:"mongo_endpoint"`
}

// RdtActuator represents the actual RDT actuator.
type RdtActuator struct {
	config RdtConfig
	tracer controller.Tracer
	k8s    kubernetes.Interface
	Cmd    *exec.Cmd
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
	CPU      float64 `json:"cpu"`
	Option   string  `json:"option"`
	Replicas int     `json:"replicas"`
}

// responseBody represents the json returned by the prediction function.
type responseBody struct {
	Val float64 `json:"val"`
}

// doQuery calls the prediction function.
func doQuery(body requestBody) float64 {
	tmp, _ := json.Marshal(body)
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Post("http://127.0.0.1:8000", "application/json", bytes.NewBuffer(tmp))
	if err != nil {
		klog.Errorf("Could not reach prediction endpoint: %s.", err)
		return -1.0
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
		return -1.0
	}
	var res responseBody
	err = json.Unmarshal(respBody, &res)
	if err != nil {
		klog.Errorf("Could not unmarshall response: %s - request: %+v - response: %s", err, body, respBody)
		return -1.0
	}
	return res.Val
}

// getResources tries to determine the allocated cpu units for the last container.
func getResources(resources map[string]int64) int64 {
	cpu := int64(0)
	index := -1
	for key, value := range resources {
		name := strings.Split(key, "_")
		containerIdx, err := strconv.Atoi(name[0])
		if err != nil {
			klog.Errorf("Failed to convert: %v", err)
			return 0
		}
		if name[1] == "cpu" && containerIdx > index && name[2] == "limits" {
			cpu = value
			index = containerIdx
		}
	}
	return cpu
}

// findStates determine a set of possible follow-up states.
func (rdt RdtActuator) findStates(start *common.State, goal *common.State, profiles map[string]common.Profile, currentOption string) ([]common.State, []float64, []planner.Action) {
	var candidates []common.State
	var utilities []float64
	var actions []planner.Action

	for _, option := range rdt.config.Options {
		newState := start.DeepCopy()
		tempSum := 0.0
		tempPredSum := 0.0
		found := true

		// predict effect of this config option...
		candidate := false
		for k := range newState.Intent.Objectives {
			if profiles[k].ProfileType != common.ProfileTypeFromText("latency") {
				// only work on latency related targets.
				continue
			}

			// required parameters.
			cpu := getResources(start.Resources)
			replicas := len(newState.CurrentPods)

			// actually predict.
			body := requestBody{
				Name:     newState.Intent.Key,
				Target:   k,
				Option:   option,
				CPU:      float64(cpu / 1000.0),
				Replicas: replicas,
			}
			predictedValue := doQuery(body)
			if predictedValue == -1.0 {
				// Predict script couldn't figure sth out -> need to skip this option.
				found = false
				break
			}

			newState.Intent.Objectives[k] = predictedValue
			if newState.Intent.Objectives[k] <= goal.Intent.Objectives[k] {
				candidate = true
			}
			tempPredSum += predictedValue
			tempSum += goal.Intent.Objectives[k]
		}

		// test if better and closer...
		if option == "None" && newState.IsBetter(goal, profiles) && found {
			// if None is good enough, go for that.
			if currentOption != "None" {
				return []common.State{newState}, []float64{0.0}, []planner.Action{{Name: rdt.Name(), Properties: map[string]string{"option": "None"}}}
			}
			return nil, nil, nil
		} else if candidate && found {
			if len(newState.Annotations) > 0 {
				newState.Annotations["rdtVisited"] = ""
			} else {
				newState.Annotations = map[string]string{"rdtVisited": ""}
			}
			utility := 0.0
			if tempPredSum < tempSum {
				utility = tempPredSum / tempSum * goal.Intent.Priority
			} else {
				utility = tempPredSum / tempSum * 1 / goal.Intent.Priority
			}
			if option != currentOption {
				candidates = append(candidates, newState)
				utilities = append(utilities, utility)
				actions = append(actions, planner.Action{Name: rdt.Name(), Properties: map[string]string{"option": option}})
			}
			if newState.IsBetter(goal, profiles) {
				break
			}
		}
	}
	return candidates, utilities, actions
}

func (rdt RdtActuator) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	klog.V(2).Infof("Finding next state for %s.", state.Intent.Key)
	currentOption := "None"
	// we do not support recursive action calls in the state graph.
	if _, found := state.Annotations[rdt.config.AnnotationName]; found {
		currentOption = state.Annotations[rdt.config.AnnotationName]
	}
	if _, found := state.Annotations["rdtVisited"]; found {
		return nil, nil, nil
	}

	// make sure we only operate on guaranteed PODs.
	for _, podState := range state.CurrentPods {
		if podState.QoSClass != "Guaranteed" {
			klog.Warningf("Targeted workload is not in guaranteed QoS class.")
			return nil, nil, nil
		}
		break // all PODs are equal.
	}

	// find a good follow-up state...
	states, utilities, actions := rdt.findStates(state, goal, profiles, currentOption)
	return states, utilities, actions
}

func (rdt RdtActuator) Perform(state *common.State, plan []planner.Action) {
	klog.V(2).Infof("%s-%s - performing plan: %v", rdt.Group(), rdt.Name(), plan)
	option := "n/a"
	for _, item := range plan {
		if item.Name == rdt.Name() {
			option = item.Properties.(map[string]string)["option"]
		}
	}
	if option == "n/a" {
		klog.V(2).Infof("Nothing to do for: %v.", state.Intent.TargetKey)
		return
	}
	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]

	// set annotation on deployment - will cause a POD restart atm; required atm as containerd/cri-o only handle setting RDT settings on POD starts.
	if state.Intent.TargetKind == "Deployment" {
		deployment, err := rdt.k8s.AppsV1().Deployments(namespace).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("failed to get deployment: %s - %v", state.Intent.TargetKey, err)
			return
		}
		newDeployment := deployment.DeepCopy()
		podAnnotations := newDeployment.Spec.Template.ObjectMeta.Annotations
		if podAnnotations == nil {
			podAnnotations = make(map[string]string)
		}
		if option != "None" {
			podAnnotations[rdt.config.AnnotationName] = option
		} else {
			delete(podAnnotations, rdt.config.AnnotationName)
		}
		_, err = rdt.k8s.AppsV1().Deployments(deployment.ObjectMeta.Namespace).Update(context.TODO(), newDeployment, metaV1.UpdateOptions{})
		if err != nil {
			klog.Errorf("failed to update deployment: %v", err)
			return
		}
	} else if state.Intent.TargetKind == "ReplicaSet" {
		replicaSet, err := rdt.k8s.AppsV1().ReplicaSets(namespace).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("failed to get replicaset: %s - %v", state.Intent.TargetKey, err)
			return
		}
		newReplicaSet := replicaSet.DeepCopy()
		podAnnotations := newReplicaSet.Spec.Template.ObjectMeta.Annotations
		if podAnnotations == nil {
			podAnnotations = make(map[string]string)
		}
		if option != "None" {
			podAnnotations[rdt.config.AnnotationName] = option
		} else {
			delete(podAnnotations, rdt.config.AnnotationName)
		}
		_, err = rdt.k8s.AppsV1().ReplicaSets(replicaSet.ObjectMeta.Namespace).Update(context.TODO(), newReplicaSet, metaV1.UpdateOptions{})
		if err != nil {
			klog.Errorf("failed to update replicaset: %v", err)
			return
		}
	}

	// TODO: add NFD/NPD requirement label.
	// TODO: set any hints for scheduler.
}

func (rdt RdtActuator) Effect(state *common.State, profiles map[string]common.Profile) {
	if rdt.config.Analytics == "None" {
		klog.V(2).Infof("Effect calculation is disabled.")
		return
	}
	klog.V(2).Infof("Triggering effect calculation for: %v", state.Intent.TargetKey)
	for name := range state.Intent.Objectives {
		if profiles[name].ProfileType != common.ProfileTypeFromText("latency") {
			continue
		}
		cmd := exec.Command(rdt.config.Interpreter, rdt.config.Analytics, state.Intent.Key, name, rdt.config.AnnotationName) //#nosec G204 -- NA
		out, err := cmd.CombinedOutput()
		if err != nil {
			klog.Errorf("Error triggering analytics script: %s - %s.", err, string(out))
		}
	}
}

// NewRdtActuator initializes a new actuator.
func NewRdtActuator(client kubernetes.Interface, tracer controller.Tracer, cfg RdtConfig) *RdtActuator {
	cmd := exec.Command(cfg.Interpreter, cfg.Prediction) //#nosec G204 -- NA
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		klog.Errorf("Could not start the prediction script: %s.", err)
	}
	time.Sleep(500 * time.Millisecond)
	klog.Infof("PID is: %d", cmd.Process.Pid)

	return &RdtActuator{
		config: cfg,
		tracer: tracer,
		k8s:    client,
		Cmd:    cmd,
	}
}
