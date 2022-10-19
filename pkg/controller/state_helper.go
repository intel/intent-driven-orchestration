package controller

import (
	"context"
	"strings"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// getPods returns information about the PODs which are part of the pod set.
func getPods(clientSet kubernetes.Interface,
	informer v1.PodInformer,
	targetKey string,
	targetKind string,
	podErrors map[string][]common.PodError) (map[string]common.PodState, []string) {
	res := map[string]common.PodState{}
	var hosts []string
	tmp := strings.Split(targetKey, "/")
	var labels *metaV1.LabelSelector
	if targetKind == "Deployment" {
		deployment, err := clientSet.AppsV1().Deployments(tmp[0]).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("Deployment '%s' could not be found: %s.", targetKey, err)
			return nil, nil
		}
		labels = deployment.Spec.Selector
	} else if targetKind == "ReplicaSet" {
		rs, err := clientSet.AppsV1().ReplicaSets(tmp[0]).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
		if err != nil {
			klog.Errorf("ReplicaSet '%s' could not be found: %s.", targetKey, err)
			return nil, nil
		}
		labels = rs.Spec.Selector
	}
	// Will ignore errors as pod list will be empty anyhow.
	selector, _ := metaV1.LabelSelectorAsSelector(labels)
	pods, _ := informer.Lister().Pods(tmp[0]).List(selector)
	for _, pod := range pods {
		podAvailability := podAvailability(podErrors[pod.Name], time.Now())
		res[pod.Name] = common.PodState{
			Resources:    nil, // TODO: pick this up!
			Annotations:  pod.ObjectMeta.Annotations,
			Availability: podAvailability,
			NodeName:     pod.Spec.NodeName,
			State:        string(pod.Status.Phase),
			QoSClass:     string(pod.Status.QOSClass),
		}
		hosts = append(hosts, pod.Spec.NodeName)
	}
	return res, hosts
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
	pods, hosts := getPods(clientSet, informer, objective.TargetKey, objective.TargetKind, podErrors)
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
		},
		CurrentPods: pods,
		CurrentData: data,
	}
	return state
}

// getDesiredState returns the desired state for an objective.
func getDesiredState(objective common.Intent) common.State {
	return common.State{Intent: objective}
}
