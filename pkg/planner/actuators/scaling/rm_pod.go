package scaling

import (
	"context"
	"strings"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

// rmPodActionName represents the name for the remove action.
const rmPodActionName = "rmPod"

// RmPodConfig represents the configuration for this actuator.
type RmPodConfig struct {
	LookBack              int    `json:"look_back"`
	MinPods               int    `json:"min_pods"`
	Port                  int    `json:"port"`
	Endpoint              string `json:"endpoint"`
	MongoEndpoint         string `json:"mongo_endpoint"`
	PluginManagerEndpoint string `json:"plugin_manager_endpoint"`
	PluginManagerPort     int    `json:"plugin_manager_port"`
}

// RmPodActuator is an actuator that can remove particular PODs.
type RmPodActuator struct {
	cfg    RmPodConfig
	tracer controller.Tracer
	core   kubernetes.Interface
}

func (rm RmPodActuator) Name() string {
	return rmPodActionName
}

func (rm RmPodActuator) Group() string {
	return scalingGroupName
}

func (rm RmPodActuator) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	var states []common.State
	var utilities []float64
	var actions []planner.Action

	var throughputObjective string
	for k := range state.Intent.Objectives {
		if profiles[k].ProfileType == common.ProfileTypeFromText("throughput") {
			throughputObjective = k
		}
	}
	if len(throughputObjective) == 0 {
		klog.Warningf("could not find an throughput related objective: %v", state.Intent.Objectives)
		return states, utilities, actions
	}

	for podName, podState := range state.CurrentPods {
		if strings.Contains(podName, "dummy@") || podState.State != "Running" {
			continue
		}
		newState := state.DeepCopy()
		delete(newState.CurrentPods, podName)

		util := 1.0
		for k := range state.Intent.Objectives {
			if profiles[k].ProfileType == common.ProfileTypeFromText("latency") {
				res, err := rm.tracer.GetEffect(state.Intent.Key, rm.Group(), k, rm.cfg.LookBack, func() interface{} {
					return &ScaleOutEffect{}
				})
				if err != nil || len(res.(*ScaleOutEffect).ReplicaRange) < 1 {
					klog.Warningf("No valid effect data found in knowledge base: %v.", res)
					return states, utilities, actions
				}
				newState.Intent.Objectives[k] = predictLatency(res.(*ScaleOutEffect).Popt, (state.Intent.Objectives[throughputObjective]*res.(*ScaleOutEffect).ThroughputScale[0])+res.(*ScaleOutEffect).ThroughputScale[1], len(newState.CurrentPods))
			} else if profiles[k].ProfileType == common.ProfileTypeFromText("availability") {
				newState.Intent.Objectives[k] = controller.PodSetAvailability(newState.CurrentPods)
				util = newState.Intent.Objectives[k]
			}
		}
		if len(newState.CurrentPods) >= rm.cfg.MinPods {
			states = append(states, newState)
			utilities = append(utilities, util*goal.Intent.Priority)
			actions = append(actions, planner.Action{Name: rm.Name(), Properties: map[string]string{"name": podName}})
		}
	}

	return states, utilities, actions
}

func (rm RmPodActuator) Perform(state *common.State, plan []planner.Action) {
	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]
	for _, item := range plan {
		if item.Name == rm.Name() {
			name := item.Properties.(map[string]string)["name"]
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err := rm.core.CoreV1().Pods(namespace).Delete(context.TODO(), name, metaV1.DeleteOptions{})
				return err
			})
			if retryErr != nil {
				klog.Errorf("failed to delete POD: %v", retryErr)
			}
		}
	}
}

func (rm RmPodActuator) Effect(_ *common.State, _ map[string]common.Profile) {
	klog.V(2).Info("Nothing to do here...")
}

// NewRmPodActuator initializes a new actuator.
func NewRmPodActuator(core kubernetes.Interface, tracer controller.Tracer, cfg RmPodConfig) *RmPodActuator {
	return &RmPodActuator{
		cfg:    cfg,
		tracer: tracer,
		core:   core,
	}
}
