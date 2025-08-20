package controller

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// resourceDelimiter defines the delimiter used to constructor the keys for the resource map.
const resourceDelimiter = "_"

// getPods returns information about the PODs & their containers which are part of the pod set. Will return information on pod states, annotations and resources.
func getPods(clientSet kubernetes.Interface,
	informer v1.PodInformer,
	targetKey string,
	targetKind string,
	podErrors map[string][]common.PodError) (map[string]common.PodState, map[string]string, map[string]int64, []string) {
	podStates := map[string]common.PodState{}
	var hosts []string
	tmp := strings.Split(targetKey, "/")
	if len(tmp) <= 1 {
		// TODO: for future release check if we can use a struct(ns, name) instead of string - check K8s.
		klog.Errorf("invalid target key")
		return nil, nil, nil, nil
	}
	var labels *metaV1.LabelSelector
	if targetKind == "Deployment" {
		deployment, err := clientSet.AppsV1().Deployments(tmp[0]).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("Deployment '%s' could not be found: %s.", targetKey, err)
			return nil, nil, nil, nil
		}
		labels = deployment.Spec.Selector
	} else if targetKind == "ReplicaSet" {
		rs, err := clientSet.AppsV1().ReplicaSets(tmp[0]).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("ReplicaSet '%s' could not be found: %s.", targetKey, err)
			return nil, nil, nil, nil
		}
		labels = rs.Spec.Selector
	}
	// Will ignore errors as pod list will be empty anyhow.
	selector, _ := metaV1.LabelSelectorAsSelector(labels)
	pods, _ := informer.Lister().Pods(tmp[0]).List(selector)
	containerResources := map[string]int64{}
	var annotations map[string]string
	for _, pod := range pods {
		if len(annotations) != len(pod.ObjectMeta.Annotations) {
			annotations = pod.ObjectMeta.Annotations
		}
		// for each container an entry is created in the map; the key holds a container index, resource name and identifier for requests and limits.
		if len(containerResources) < 1 {
			for i, container := range pod.Spec.Containers {
				for name, requests := range container.Resources.Requests {
					containerResources[strings.Join([]string{strconv.Itoa(i), name.String(), "requests"}, resourceDelimiter)] = requests.MilliValue()
				}
				for name, limits := range container.Resources.Limits {
					containerResources[strings.Join([]string{strconv.Itoa(i), name.String(), "limits"}, resourceDelimiter)] = limits.MilliValue()
				}
			}
		}
		podAvailability := podAvailability(podErrors[pod.Name], time.Now())
		podStates[pod.Name] = common.PodState{
			Availability: podAvailability,
			NodeName:     pod.Spec.NodeName,
			State:        string(pod.Status.Phase),
			QoSClass:     string(pod.Status.QOSClass),
		}
		hosts = append(hosts, pod.Spec.NodeName)
	}
	return podStates, annotations, containerResources, hosts
}

// getCurrentState returns the current state for an objective.
func getCurrentState(
	cfg common.ControllerConfig,
	clientSet kubernetes.Interface,
	informer v1.PodInformer,
	objective common.Intent,
	podErrors map[string][]common.PodError,
	profiles map[string]common.Profile) common.State {
	currentObjectives := map[string]float64{}

	// get current measurements
	pods, annotations, resources, hosts := getPods(clientSet, informer, objective.TargetKey, objective.TargetKind, podErrors)
	for item := range objective.Objectives {
		profile := profiles[item]
		if profile == (common.Profile{}) {
			klog.Warningf("Non-existence profile references for objective target: %s.", item)
			continue
		}
		if profile.ProfileType == common.ProfileTypeFromText("availability") {
			currentObjectives[item] = PodSetAvailability(pods)
		} else {
			currentObjectives[item] = doQuery(profile, objective)
		}
	}

	// bring in telemetry info
	data := map[string]map[string]float64{}
	for _, metric := range cfg.Metrics {
		tmp := getHostTelemetry(cfg.TelemetryEndpoint, metric.Query, hosts, cfg.HostField)
		data[metric.Name] = tmp
	}

	// and finally come up with the state.
	state := common.State{
		Intent: common.Intent{
			Key:        objective.Key,
			Priority:   objective.Priority,
			TargetKey:  objective.TargetKey,
			TargetKind: objective.TargetKind,
			Objectives: currentObjectives,
			// For current state we'll not carry the tolerations.
		},
		CurrentPods: pods,
		CurrentData: data,
		Resources:   resources,
		Annotations: annotations,
	}
	return state
}

// getDesiredState returns the desired state for an objective.
func getDesiredState(objective common.Intent) common.State {
	return common.State{Intent: objective}
}
