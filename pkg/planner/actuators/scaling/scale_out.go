package scaling

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

// scaleOutActionName represents the name of the scale out action.
const scaleOutActionName = "scaleOut"

// scalingGroupName represents the name for the scaling related set of actions.
const scalingGroupName = "scaling"

// getInt32Pointer return a ref to an int32.
func getInt32Pointer(value int32) *int32 {
	val := value
	return &val
}

// ScaleOutConfig describes the configuration for this actuator.
type ScaleOutConfig struct {
	PythonInterpreter      string  `json:"interpreter"`
	Script                 string  `json:"analytics_script"`
	MaxPods                int     `json:"max_pods"`
	LookBack               int     `json:"look_back"`
	MaxProActiveScaleOut   int     `json:"max_proactive_scale_out"`
	ProActiveLatencyFactor float64 `json:"proactive_latency_factor"`
	Endpoint               string  `json:"endpoint"`
	Port                   int     `json:"port"`
	PluginManagerEndpoint  string  `json:"plugin_manager_endpoint"`
	PluginManagerPort      int     `json:"plugin_manager_port"`
	MongoEndpoint          string  `json:"mongo_endpoint"`
}

// ScaleOutEffect describes the data that is stored in the knowledge base.
type ScaleOutEffect struct {
	// Never ever think about making these non-public! Needed for marshalling this struct.
	ThroughputRange  [2]float64
	ThroughputScale  [2]float64
	ReplicaRange     [2]int
	Popt             [4]float64
	TrainingFeatures [2]string
	TargetFeature    string
	Image            string
}

// ScaleOutActuator is an actuator supporting horizontal scaling.
type ScaleOutActuator struct {
	cfg    ScaleOutConfig
	tracer controller.Tracer
	apps   kubernetes.Interface
}

func (scale ScaleOutActuator) Name() string {
	return scaleOutActionName
}

func (scale ScaleOutActuator) Group() string {
	return scalingGroupName
}

// averageAvailability calculates the average availability for a set of PODs.
func averageAvailability(pods map[string]common.PodState) float64 {
	res := 0.0
	i := 0
	for _, v := range pods {
		res += v.Availability
		i++
	}
	return res / float64(i) // should never be 0 - that'll mean it would be a state w/o PODs.
}

// predictLatency uses the knowledge base to forecast the latency.
func predictLatency(popt [4]float64, throughput float64, numPods int) float64 {
	// TODO: predict future throughput.
	if numPods == 0 || throughput == 0 {
		return 0.0
	}
	return (popt[0] * math.Exp(popt[1]*throughput)) / (popt[2] * math.Exp(popt[3]*throughput*float64(numPods)))
}

// findStates tries to determine the best possible # of replicas.
func (scale ScaleOutActuator) findStates(state *common.State, goal *common.State, throughputObjective string, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action, error) {
	last := state
	var candidates []common.State
	var utilities []float64
	var actions []planner.Action
	for i := len(state.CurrentPods) - 1; i < scale.cfg.MaxPods; i++ {
		found := false
		newState := last.DeepCopy()
		newState.CurrentPods["dummy@"+strconv.Itoa(i)] = common.PodState{Availability: averageAvailability(newState.CurrentPods)}
		for k := range state.Intent.Objectives {
			if profiles[k].ProfileType == common.ProfileTypeFromText("latency") {
				res, err := scale.tracer.GetEffect(state.Intent.Key, scale.Group(), k, scale.cfg.LookBack, func() interface{} {
					return &ScaleOutEffect{}
				})
				if err != nil {
					return nil, nil, nil, fmt.Errorf("no valid effect data found in knowledge base: %s - %v", err, res)
				}
				if len(newState.CurrentPods) > res.(*ScaleOutEffect).ReplicaRange[1] {
					return nil, nil, nil, fmt.Errorf("scaling out further won't help - known replica Range: %v", res.(*ScaleOutEffect).ReplicaRange)
				}
				newState.Intent.Objectives[k] = predictLatency(res.(*ScaleOutEffect).Popt, (state.Intent.Objectives[throughputObjective]*res.(*ScaleOutEffect).ThroughputScale[0])+res.(*ScaleOutEffect).ThroughputScale[1], len(newState.CurrentPods))
				if newState.Intent.Objectives[k] <= goal.Intent.Objectives[k] {
					found = true
				}
			} else if profiles[k].ProfileType == common.ProfileTypeFromText("availability") {
				newState.Intent.Objectives[k] = controller.PodSetAvailability(newState.CurrentPods)
				if newState.Intent.Objectives[k] >= goal.Intent.Objectives[k] {
					found = true
				}
			}
		}
		// if at least one objective is improved by scaling mark it as a candidate; or...
		if found {
			candidates = append(candidates, newState)
			utilities = append(utilities, 0.9+(float64(len(newState.CurrentPods))/float64(scale.cfg.MaxPods))*(1.0/goal.Intent.Priority))
			actions = append(actions, planner.Action{
				Name: scale.Name(), Properties: map[string]int64{"factor": int64(len(newState.CurrentPods) - len(state.CurrentPods))}})
		}
		// ... when all are satisfied we can stop.
		if newState.IsBetter(goal, profiles) {
			break
		}
		last = &newState
	}
	if len(candidates) == 0 {
		return nil, nil, nil, fmt.Errorf("could not find a follow-up state for: %v", state)
	}
	return candidates, utilities, actions, nil
}

func (scale ScaleOutActuator) NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	if state.IsBetter(goal, profiles) {
		return nil, nil, nil
	}

	var throughputObjective string
	for k := range state.Intent.Objectives {
		if profiles[k].ProfileType == common.ProfileTypeFromText("throughput") {
			throughputObjective = k
		}
	}
	if len(throughputObjective) == 0 {
		klog.Warningf("Could not find an throughput related objective: %v.", state.Intent.Objectives)
		return nil, nil, nil
	}

	states, utils, actions, err := scale.findStates(state, goal, throughputObjective, profiles)
	if err != nil {
		klog.Warningf("Could not determine scale out factor: %s", err)
		if _, ok := state.CurrentPods["proactiveTemp"]; !ok && len(state.CurrentPods) < scale.cfg.MaxProActiveScaleOut {
			klog.Infof("Trying a proactive scale-out to see if I can learn sth.")
			tempState := state.DeepCopy()
			tempState.CurrentPods["proactiveTemp"] = common.PodState{Availability: averageAvailability(tempState.CurrentPods)}
			for name := range tempState.Intent.Objectives {
				if profiles[name].ProfileType == common.ProfileTypeFromText("latency") {
					tempState.Intent.Objectives[name] *= scale.cfg.ProActiveLatencyFactor
				}
			}
			return []common.State{tempState}, []float64{0.1}, []planner.Action{{Name: scale.Name(), Properties: map[string]int64{"factor": 1, "proactive": 1}}}
		}
		return nil, nil, nil
	}
	return states, utils, actions
}

func (scale ScaleOutActuator) Perform(state *common.State, plan []planner.Action) {
	// calculate the scale factor
	var factor int64
	factor = 0
	for _, item := range plan {
		if item.Name == rmPodActionName {
			factor--
		} else if item.Name == scaleOutActionName {
			factor += item.Properties.(map[string]int64)["factor"]
		}
	}

	// set replicas.
	if !strings.Contains(state.Intent.TargetKey, "/") && strings.Count(state.Intent.TargetKey, "/") != 1 {
		klog.Errorf("not a valid format")
		return
	}
	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]
	name := tmp[1]
	if state.Intent.TargetKind == "Deployment" {
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			res, err := scale.apps.AppsV1().Deployments(namespace).Get(context.TODO(), name, metaV1.GetOptions{})
			if err != nil {
				klog.Errorf("failed to get latest version of: %v", err)
				return err
			}
			// conversion to int32 is ok - as we have a MaxPods defined
			res.Spec.Replicas = getInt32Pointer(*res.Spec.Replicas + int32(factor)) // #nosec G115
			if *res.Spec.Replicas > 0 {
				_, updateErr := scale.apps.AppsV1().Deployments(namespace).Update(context.TODO(), res, metaV1.UpdateOptions{})
				return updateErr
			}
			return nil
		})
		if retryErr != nil {
			klog.Errorf("failed to update: %v", retryErr)
		}
	} else if state.Intent.TargetKind == "ReplicaSet" {
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			res, err := scale.apps.AppsV1().ReplicaSets(namespace).Get(context.TODO(), name, metaV1.GetOptions{})
			if err != nil {
				klog.Errorf("failed to get latest version of: %v", err)
				return err
			}
			// conversion to int32 is ok - as we have a MaxPods defined
			res.Spec.Replicas = getInt32Pointer(*res.Spec.Replicas + int32(factor)) // #nosec G115
			if *res.Spec.Replicas > 0 {
				_, updateErr := scale.apps.AppsV1().ReplicaSets(namespace).Update(context.TODO(), res, metaV1.UpdateOptions{})
				return updateErr
			}
			return nil
		})
		if retryErr != nil {
			klog.Errorf("failed to update: %v", retryErr)
		}
	}
}

func (scale ScaleOutActuator) Effect(state *common.State, profiles map[string]common.Profile) {
	if scale.cfg.Script == "None" {
		klog.V(2).Infof("Effect calculation is disabled - will not run analytics.")
		return
	}
	throughputObjective := ""
	var latencyObjectives []string

	// need to check if we have a throughput related objective.
	for k := range state.Intent.Objectives {
		if profiles[k].ProfileType == common.ProfileTypeFromText("latency") {
			latencyObjectives = append(latencyObjectives, k)
		} else if profiles[k].ProfileType == common.ProfileTypeFromText("throughput") {
			throughputObjective = k
		}
	}

	if throughputObjective != "" {
		// for all latency related objectives we (re-)analyse what the effect of scaling out is.
		for _, objective := range latencyObjectives {
			cmd := exec.Command(scale.cfg.PythonInterpreter, scale.cfg.Script, state.Intent.Key, objective, throughputObjective) //#nosec G204 -- NA
			out, err := cmd.CombinedOutput()
			if err != nil {
				klog.Errorf("Error triggering analytics script: %s - %s.", err, string(out))
			}
			klog.V(2).Infof("Script output was: %v", string(out))
		}
	} else {
		klog.Info("No throughput related objective defined - skipping analytics!")
	}
}

// NewScaleOutActuator initializes a new actuator.
func NewScaleOutActuator(apps kubernetes.Interface, tracer controller.Tracer, cfg ScaleOutConfig) *ScaleOutActuator {
	return &ScaleOutActuator{
		cfg:    cfg,
		tracer: tracer,
		apps:   apps,
	}
}
