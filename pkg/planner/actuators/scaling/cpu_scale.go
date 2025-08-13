package scaling

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"os/exec"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// delimiter for the key entries in the resources hashmap.
const delimiter = "_"

// actionName represents the name of the action.
const actionName = "scaleCPU"

// groupName represents the name for the scaling related set of actions.
const groupName = "vertical_scaling"

// CPUScaleConfig describes the configuration for this actuator.
// TODO: need to validate the new parameters such as MinCore, MaxCore, CPUIncrement, and MaxProActiveCPU
type CPUScaleConfig struct {
	PythonInterpreter          string  `json:"interpreter"`
	Script                     string  `json:"analytics_script"`
	CPUMax                     int64   `json:"cpu_max"`
	CPURounding                int64   `json:"cpu_rounding"`
	CPUSafeGuardFactor         float64 `json:"cpu_safeguard_factor"`
	BoostFactor                float64 `json:"boost_factor"`
	MaxProActiveCPU            int64   `json:"max_proactive_cpu"`
	ProActiveLatencyPercentage float64 `json:"proactive_latency_percentage"`
	LookBack                   int     `json:"look_back"`
	Endpoint                   string  `json:"endpoint"`
	Port                       int     `json:"port"`
	PluginManagerEndpoint      string  `json:"plugin_manager_endpoint"`
	PluginManagerPort          int     `json:"plugin_manager_port"`
	MongoEndpoint              string  `json:"mongo_endpoint"`
}

// CPUScaleEffect describes the data that is stored in the knowledge base.
type CPUScaleEffect struct {
	// Never ever think about making these non-public! Needed for marshalling this struct.
	LatencyRange     [2]float64
	CPURange         [2]float64
	Popts            [][3]float64
	TrainingFeatures [1]string
	TargetFeature    string
	Image            string
}

// CPUScaleActuator is an actuator supporting the resource scaling.
type CPUScaleActuator struct {
	cfg          CPUScaleConfig
	tracer       controller.Tracer
	apps         kubernetes.Interface
	cpuIncrement int64
}

func (cs CPUScaleActuator) Name() string {
	return actionName
}

func (cs CPUScaleActuator) Group() string {
	return groupName
}

// predictLatency uses the knowledge base to calculate the latency. It does use the parameters popt
// that are obtained when sum of the squared residuals which is minimized. More info in:
// https://docs.scipy.org/doc/scipy/reference/generated/scipy.optimize.curve_fit.html
func (cs CPUScaleActuator) predictLatency(popt [3]float64, limCPU float64) float64 {
	result := popt[0]*math.Exp(-popt[1]*(limCPU/1000)) + popt[2]
	return result
}

// getResourceValues return the cpu resources associated with the last container of a POD.
func getResourceValues(state *common.State) (int64, int) {
	cpuRequest := int64(0)
	cpuLimit := int64(0)
	lastIndex := -1
	for key, value := range state.Resources {
		items := strings.Split(key, delimiter)
		if len(items) != 3 {
			continue
		}
		index, err := strconv.Atoi(items[0])
		if err != nil {
			klog.Errorf("Failed to convert index: %v", err)
			continue
		}
		if items[1] != "cpu" {
			continue
		}
		if index > lastIndex {
			cpuRequest = 0
			cpuLimit = 0
			lastIndex = index
		}
		if index == lastIndex {
			if items[2] == "requests" {
				cpuRequest = value
			} else if items[2] == "limits" {
				cpuLimit = value
			}
		}
	}
	if lastIndex == -1 {
		return 0, -1
	}
	if cpuLimit >= cpuRequest {
		return cpuLimit, lastIndex
	}
	return cpuRequest, lastIndex
}

// setResourceValues tweaks the resource requests limits on the workload.
func (cs CPUScaleActuator) setResourceValues(state *common.State, newValue int) {
	tmp := strings.Split(state.Intent.TargetKey, "/")
	namespace := tmp[0]

	if state.Intent.TargetKind == "Deployment" {
		client := cs.apps.AppsV1().Deployments(namespace)
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			deployment, err := client.Get(context.TODO(), tmp[1], metaV1.GetOptions{})
			if err != nil {
				klog.Errorf("Failed to get latest version of Deployment: %v", err)
				return nil
			}
			updatedDeployment := deployment.DeepCopy()

			request := resource.NewMilliQuantity(int64(newValue), resource.DecimalSI).DeepCopy()
			if len(updatedDeployment.Spec.Template.Spec.Containers[len(updatedDeployment.Spec.Template.Spec.Containers)-1].Resources.Requests) == 0 {
				updatedDeployment.Spec.Template.Spec.Containers[len(updatedDeployment.Spec.Template.Spec.Containers)-1].Resources.Requests = make(map[v1.ResourceName]resource.Quantity)
			}
			updatedDeployment.Spec.Template.Spec.Containers[len(updatedDeployment.Spec.Template.Spec.Containers)-1].Resources.Requests["cpu"] = request

			if cs.cfg.BoostFactor >= 1.0 {
				limit := resource.NewMilliQuantity(int64(float64(newValue)*cs.cfg.BoostFactor), resource.DecimalSI).DeepCopy()
				if len(updatedDeployment.Spec.Template.Spec.Containers[len(updatedDeployment.Spec.Template.Spec.Containers)-1].Resources.Limits) == 0 {
					updatedDeployment.Spec.Template.Spec.Containers[len(updatedDeployment.Spec.Template.Spec.Containers)-1].Resources.Limits = make(map[v1.ResourceName]resource.Quantity)
				}
				updatedDeployment.Spec.Template.Spec.Containers[len(updatedDeployment.Spec.Template.Spec.Containers)-1].Resources.Limits["cpu"] = limit
			}

			_, updateErr := client.Update(context.TODO(), updatedDeployment, metaV1.UpdateOptions{})
			return updateErr
		})
		if retryErr != nil {
			klog.Errorf("Update of deployment %s failed: %v.", state.Intent.TargetKey, retryErr)
		}
	} else if state.Intent.TargetKind == "ReplicaSet" {
		client := cs.apps.AppsV1().ReplicaSets(namespace)
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			replicaSet, err := client.Get(context.TODO(), tmp[1], metaV1.GetOptions{})
			if err != nil {
				klog.Errorf("Failed to get latest version of ReplicaSet: %v", err)
				return nil
			}
			updatedReplicaSet := replicaSet.DeepCopy()

			request := resource.NewMilliQuantity(int64(newValue), resource.DecimalSI).DeepCopy()
			if len(updatedReplicaSet.Spec.Template.Spec.Containers[len(updatedReplicaSet.Spec.Template.Spec.Containers)-1].Resources.Requests) == 0 {
				updatedReplicaSet.Spec.Template.Spec.Containers[len(updatedReplicaSet.Spec.Template.Spec.Containers)-1].Resources.Requests = make(map[v1.ResourceName]resource.Quantity)
			}
			updatedReplicaSet.Spec.Template.Spec.Containers[len(updatedReplicaSet.Spec.Template.Spec.Containers)-1].Resources.Requests["cpu"] = request

			if cs.cfg.BoostFactor >= 1.0 {
				limit := resource.NewMilliQuantity(int64(float64(newValue)*cs.cfg.BoostFactor), resource.DecimalSI).DeepCopy()
				if len(updatedReplicaSet.Spec.Template.Spec.Containers[len(updatedReplicaSet.Spec.Template.Spec.Containers)-1].Resources.Limits) == 0 {
					updatedReplicaSet.Spec.Template.Spec.Containers[len(updatedReplicaSet.Spec.Template.Spec.Containers)-1].Resources.Limits = make(map[v1.ResourceName]resource.Quantity)
				}
				updatedReplicaSet.Spec.Template.Spec.Containers[len(updatedReplicaSet.Spec.Template.Spec.Containers)-1].Resources.Limits["cpu"] = limit
			}

			_, updateErr := client.Update(context.TODO(), updatedReplicaSet, metaV1.UpdateOptions{})
			return updateErr
		})
		if retryErr != nil {
			klog.Errorf("Update of ReplicaSet %s failed: %v.", state.Intent.TargetKey, retryErr)
		}
	}
}

// roundUpCores returns the next better cpu allocation.
func roundUpCores(n int64, fraction int64) int64 {
	a := (n / fraction) * fraction
	b := a + fraction
	return b
}

// getScalingEffects return all entries in the knowledgebase for latency related objectives.
func (cs CPUScaleActuator) getScalingEffects(intent common.Intent, profiles map[string]common.Profile) (map[string]*CPUScaleEffect, error) {
	result := map[string]*CPUScaleEffect{}
	for key := range intent.Objectives {
		if profiles[key].ProfileType == common.ProfileTypeFromText("latency") {
			res, err := cs.tracer.GetEffect(intent.Key, cs.Group(), key, cs.cfg.LookBack, func() interface{} {
				return &CPUScaleEffect{}
			})
			if res == nil || err != nil {
				return nil, fmt.Errorf("could not retrieve information from knowledge base for: %s", key)
			}
			result[key] = res.(*CPUScaleEffect)
		}
	}
	return result, nil
}

// findStates tries to determine the best possible state for each latency related objective.
func (cs CPUScaleActuator) findStates(
	state *common.State,
	goal *common.State,
	currentCPU int64,
	containerIndex int,
	profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action, error) {
	var candidates []common.State
	var utilities []float64
	var actions []planner.Action

	// get insights from the knowledge base.
	effects, err := cs.getScalingEffects(state.Intent, profiles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not retrieve information from knowledge base for: %s", state.Intent.TargetKey)
	}

	// construct a set of follow-up states.
	for k := range state.Intent.Objectives {
		if profiles[k].ProfileType == common.ProfileTypeFromText("latency") {
			newState := state.DeepCopy()
			latency := goal.Intent.Objectives[k] * cs.cfg.CPUSafeGuardFactor
			popts := effects[k].Popts
			newCPUValue := int64(0)
			for _, popt := range popts {
				if goal.Intent.Objectives[k] < popt[2] {
					klog.Warningf("the model cannot handle this case - aborting for: %s", state.Intent.TargetKey)
					continue
				}
				latency := goal.Intent.Objectives[k] * cs.cfg.CPUSafeGuardFactor
				newState.Intent.Objectives[k] = latency
				cpuValue := int64(-(1 / popt[1]) * math.Log((latency-popt[2])/popt[0]) * 1000)
				if cpuValue > newCPUValue {
					newCPUValue = roundUpCores(cpuValue, cs.cfg.CPURounding)
					break
				}
			}

			if newCPUValue != currentCPU && newCPUValue > 0 {
				// forecast the effect of vertical scaling.
				for objectiveKey := range effects {
					newState.Intent.Objectives[objectiveKey] = latency
				}

				// resources can be nil so need a quick check here.
				// setting requests equal to limits leads to a guaranteed POD QoS.
				for name, pod := range newState.CurrentPods {
					pod.QoSClass = "BestEffort"
					if cs.cfg.BoostFactor == 1.0 {
						pod.QoSClass = "Guaranteed"
					} else if cs.cfg.BoostFactor > 1.0 {
						pod.QoSClass = "Burstable"
					}
					newState.CurrentPods[name] = pod
				}

				if newState.Resources == nil {
					newState.Resources = make(map[string]int64)
				}
				newState.Resources[strings.Join([]string{strconv.Itoa(containerIndex), "cpu", "requests"}, delimiter)] = newCPUValue
				if cs.cfg.BoostFactor >= 1.0 {
					newState.Resources[strings.Join([]string{strconv.Itoa(containerIndex), "cpu", "limits"}, delimiter)] = int64(float64(newCPUValue) * cs.cfg.BoostFactor)
				}
				newState.CurrentData[cs.Name()] = map[string]float64{cs.Name(): 1}

				// utility function.
				utility := float64(newCPUValue) / float64(cs.cfg.CPUMax)
				if newCPUValue > currentCPU {
					utility *= 1.0 / goal.Intent.Priority
				}

				// the associated action.
				action := planner.Action{Name: cs.Name(), Properties: map[string]int64{"value": newCPUValue}}

				candidates = append(candidates, newState)
				utilities = append(utilities, utility)
				actions = append(actions, action)
			}
		}
	}
	return candidates, utilities, actions, nil
}

// proactiveScaling adds a state based on hypothetical improvement on the objectives.
func (cs CPUScaleActuator) proactiveScaling(
	state *common.State,
	goal *common.State,
	currentCPU int64,
	profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {

	if _, ok := state.CurrentPods["proactiveResourceAlloc"]; !ok {
		tempState := state.DeepCopy()
		tempState.CurrentPods["proactiveResourceAlloc"] = common.PodState{
			Availability: averageAvailability(tempState.CurrentPods),
		}
		tempState.CurrentData[cs.Name()] = map[string]float64{cs.Name(): 1}

		newCPULim := int64(0)
		for name, value := range tempState.Intent.Objectives {
			if profiles[name].ProfileType == common.ProfileTypeFromText("latency") {
				delta := value - goal.Intent.Objectives[name]
				incr := cs.cpuIncrement
				if delta < 0.0 {
					// When we subtract we add a bit of randomness; this will prevent oscillating systems.
					incr = -cs.cpuIncrement + rand.Int63n(cs.cpuIncrement/2) // #nosec G404 -- pseudo random will do.
				}
				// In the future, we should support dynamic factors for scaling; for example based on the current state's distance to the desired state.
				tmp := currentCPU + incr
				if tmp > cs.cfg.MaxProActiveCPU || tmp < cs.cpuIncrement {
					klog.Warningf("This would push the resource allocations over the limits for workload: %s - %v.", state.Intent.TargetKey, tmp)
					return nil, nil, nil
				} else if tmp > newCPULim {
					newCPULim = tmp
				}
				// this assumes linear behaviour.
				if delta > 0.0 {
					tempState.Intent.Objectives[name] -= tempState.Intent.Objectives[name] * cs.cfg.ProActiveLatencyPercentage
				} else {
					tempState.Intent.Objectives[name] += tempState.Intent.Objectives[name] * cs.cfg.ProActiveLatencyPercentage
				}
			}
		}
		if newCPULim != currentCPU {
			actionPlan := []planner.Action{
				{
					Name: cs.Name(),
					Properties: map[string]int64{
						"value":     newCPULim,
						"proactive": 1,
					},
				},
			}
			return []common.State{tempState}, []float64{0.1}, actionPlan
		}
	}
	return nil, nil, nil
}

func (cs CPUScaleActuator) NextState(state *common.State, goal *common.State,
	profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	// we don't need to try this multiple times in a single planning cycle.
	if _, ok := state.CurrentData[cs.Name()]; ok {
		return nil, nil, nil
	}
	// we don't need to do anything if there are no PODs.
	if len(state.CurrentPods) == 0 {
		return nil, nil, nil
	}
	// let's find follow-up states.
	currentValue, containerIndex := getResourceValues(state)
	newStates, utilities, actions, err := cs.findStates(state, goal, currentValue, containerIndex, profiles)
	if err == nil && len(newStates) > 0 {
		return newStates, utilities, actions
	}
	// if the actuator is allowed to proactively scale - let's try that.
	if cs.cfg.MaxProActiveCPU > 0.0 {
		// if no state was found or an error is returned, the proactive from NextState is actioned.
		klog.V(2).Infof("Proactive mode is enabled - will try to do sth for: %s.", state.Intent.TargetKey)
		proactiveState, proactiveUtility, proactivePlan := cs.proactiveScaling(state, goal, currentValue, profiles)
		return proactiveState, proactiveUtility, proactivePlan
	}
	klog.Warningf("Could not find (better) next state for %s; err was: %v.", state.Intent.TargetKey, err)
	return nil, nil, nil
}

func (cs CPUScaleActuator) Perform(state *common.State, plan []planner.Action) {
	for _, item := range plan {
		if item.Name == actionName {
			a := item.Properties.(map[string]int64)
			if val, ok := a["value"]; ok {
				cs.setResourceValues(state, int(val))
			}
			break
		}
	}
}

func (cs CPUScaleActuator) Effect(state *common.State, profiles map[string]common.Profile) {
	if cs.cfg.Script == "None" {
		klog.V(2).Infof("Effect calculation is disabled - will not run analytics.")
		return
	}

	var latencyObjectives []string
	for k := range state.Intent.Objectives {
		if profiles[k].ProfileType == common.ProfileTypeFromText("latency") {
			latencyObjectives = append(latencyObjectives, k)
		}
	}

	// for all latency related objectives we (re-)analyse what the effect of scaling out is.
	for _, objective := range latencyObjectives {
		cmd := exec.Command(cs.cfg.PythonInterpreter,
			cs.cfg.Script, state.Intent.Key, objective) //#nosec G204 -- NA
		out, err := cmd.CombinedOutput()
		if err != nil {
			klog.Errorf("Error triggering analytics script: %s - %s.", err, string(out))
		}
		klog.V(2).Infof("Script output was: %v", string(out))
	}
}

// NewCPUScaleActuator initializes a new actuator.
func NewCPUScaleActuator(apps kubernetes.Interface, tracer controller.Tracer, cfg CPUScaleConfig) *CPUScaleActuator {
	return &CPUScaleActuator{
		cfg:          cfg,
		tracer:       tracer,
		apps:         apps,
		cpuIncrement: cfg.MaxProActiveCPU / 10,
	}
}
