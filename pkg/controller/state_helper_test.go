package controller

import (
	"strconv"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/intel/intent-driven-orchestration/pkg/common"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
)

// init makes sure we mock the http requests.
func init() {
	Client = &MockClient{}
}

// createDummies creates a deployment with a set of pods.
func createDummies(targetKind string, selector map[string]string, nPods int) (runtime.Object, []*coreV1.Pod) {
	var pods []*coreV1.Pod
	for i := 0; i < nPods; i++ {
		pod := &coreV1.Pod{
			TypeMeta: metaV1.TypeMeta{APIVersion: coreV1.SchemeGroupVersion.String()},
			ObjectMeta: metaV1.ObjectMeta{
				Name:        "my-deployment-" + strconv.Itoa(i),
				Namespace:   metaV1.NamespaceDefault,
				Labels:      selector,
				Annotations: map[string]string{"sample-annotation": "hello"},
			},
			Spec: coreV1.PodSpec{
				NodeName: "node0",
				Containers: []coreV1.Container{
					{
						Resources: coreV1.ResourceRequirements{
							Requests: map[coreV1.ResourceName]resource.Quantity{"foo": resource.MustParse("2")},
							Limits:   map[coreV1.ResourceName]resource.Quantity{"foo": resource.MustParse("2")},
						},
					},
					{
						Resources: coreV1.ResourceRequirements{
							Requests: map[coreV1.ResourceName]resource.Quantity{"bar": resource.MustParse("100Mi")},
							Limits:   map[coreV1.ResourceName]resource.Quantity{"bar": resource.MustParse("1000Mi")},
						},
					},
				},
			},
		}
		pods = append(pods, pod)
	}
	var res runtime.Object
	if targetKind == "Deployment" {
		res = &appsV1.Deployment{
			TypeMeta: metaV1.TypeMeta{APIVersion: appsV1.SchemeGroupVersion.String()},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-deployment",
				Namespace: metaV1.NamespaceDefault,
			},
			Spec: appsV1.DeploymentSpec{
				Selector: &metaV1.LabelSelector{MatchLabels: selector},
			},
		}
	} else {
		res = &appsV1.ReplicaSet{
			TypeMeta: metaV1.TypeMeta{APIVersion: appsV1.SchemeGroupVersion.String()},
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "my-deployment",
				Namespace: metaV1.NamespaceDefault,
			},
			Spec: appsV1.ReplicaSetSpec{
				Selector: &metaV1.LabelSelector{MatchLabels: selector},
			},
		}
	}
	return res, pods
}

// k8sShim enables us to abstract from k8s for testing.
func k8sShim(podSet runtime.Object, pods []*coreV1.Pod) (kubernetes.Interface, v1.PodInformer) {
	objects := []runtime.Object{podSet}
	for _, pod := range pods {
		objects = append(objects, pod)
	}
	client := fake.NewSimpleClientset(objects...)
	fakeWatch := watch.NewFake()
	client.PrependWatchReactor("pods", core.DefaultWatchReactor(fakeWatch, nil))
	informer := informers.NewSharedInformerFactory(client, func() time.Duration { return 0 }())
	for _, pod := range pods {
		err := informer.Core().V1().Pods().Informer().GetIndexer().Add(pod)
		if err != nil {
			klog.Fatal(err)
		}
	}
	return client, informer.Core().V1().Pods()
}

// Tests for success.

// TestGetPodsForSuccess tests for success.
func TestGetPodsForSuccess(_ *testing.T) {
	deployment, pods := createDummies("Deployment", map[string]string{"foo": "bar"}, 1)
	podErrors := map[string][]common.PodError{}
	client, informer := k8sShim(deployment, pods)
	getPods(client, informer, "default/my-deployment", "Deployment", podErrors)
}

// TestGetDesiredStateForSuccess test for success.
func TestGetDesiredStateForSuccess(_ *testing.T) {
	objective := common.Intent{
		Objectives: map[string]float64{"P99compliance": 100.0},
	}
	getDesiredState(objective)
}

// Tests for failure.

func TestGetPodsForFailure(t *testing.T) {
	deployment, pods := createDummies("Deployment", map[string]string{"foo": "bar"}, 1)
	podErrors := map[string][]common.PodError{}
	client, informer := k8sShim(deployment, pods)
	res, _, _, _ := getPods(client, informer, "default/function", "Deployment", podErrors)
	if res != nil {
		t.Errorf("Result should have been nil! - was: %v", res)
	}
	res, _, _, _ = getPods(client, informer, "default/function", "ReplicaSet", podErrors)
	if res != nil {
		t.Errorf("Result should have been nil! - was: %v", res)
	}
}

// Tests for sanity.

// TestGetPodsForSanity tests for sanity.
func TestGetPodsForSanity(t *testing.T) {
	deployment, pods := createDummies("Deployment", map[string]string{"foo": "bar"}, 1)
	podErrors := map[string][]common.PodError{}
	client, informer := k8sShim(deployment, pods)
	podStates, annotations, resources, hosts := getPods(client, informer, "default/my-deployment", "Deployment", podErrors)
	if len(podStates) != len(hosts) {
		t.Errorf("All results should have the same length: %d, %d.", len(podStates), len(hosts))
	}
	if _, ok := podStates["my-deployment-0"]; !ok {
		t.Errorf("Pod should have been included: %v", podStates)
	}
	if _, ok := annotations["sample-annotation"]; !ok {
		t.Errorf("Annotation should have been set - was: %v", annotations)
	}
	if len(resources) != 4 || resources["0_foo_requests"] != 2000 || resources["1_bar_limits"] != 1048576000000 {
		t.Errorf("Expected 4 resoure entries, one with foo another with bar resource requests & limits - was: %+v", resources)
	}
	if hosts[0] != "node0" {
		t.Errorf("Host 0 should have been node0 - was: %v", hosts[0])
	}

	// ReplicaSet.
	replicaSet, pods := createDummies("ReplicaSet", map[string]string{"foo": "bar"}, 1)
	client, informer = k8sShim(replicaSet, pods)
	podStates, _, _, hosts = getPods(client, informer, "default/my-deployment", "ReplicaSet", podErrors)
	if len(podStates) != len(hosts) {
		t.Errorf("All results should have the same length: %d, %d.", len(podStates), len(hosts))
	}
}

// TestGetCurrentStateForSanity tests for sanity
func TestGetCurrentStateForSanity(t *testing.T) {
	responseBody := "{\"data\": {\"result\": [{\"metric\": {\"exported_instance\": \"node0\"}, \"value\": [1645019125.000, \"10.0\"]}]}}"
	MockResponse(responseBody, 200)

	cfg := common.ControllerConfig{
		HostField: "exported_instance",
		Metrics: []struct {
			Name  string `json:"name,omitempty"`
			Query string `json:"query,omitempty"`
		}([]struct {
			Name  string
			Query string
		}{{"bla", ""}}),
	}
	deployment, pods := createDummies("Deployment", map[string]string{"app": "nginx"}, 1)
	client, informer := k8sShim(deployment, pods)
	objective := common.Intent{
		TargetKey:  "default/my-deployment",
		TargetKind: "Deployment",
		Objectives: map[string]float64{
			"default/p99latency":   0.1,
			"default/availability": 0.99,
			"default/no-profile":   100,
		},
	}
	created, _ := time.Parse(time.RFC3339, "2022-02-16T10:00:00Z")
	start, _ := time.Parse(time.RFC3339, "2022-02-16T11:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2022-02-16T11:00:30Z")
	errors := map[string][]common.PodError{
		"my-deployment-0": {{Start: start, End: end, Created: created}},
	}
	profiles := map[string]common.Profile{
		"default/p99latency":   {Query: "", ProfileType: common.ProfileTypeFromText("latency"), Minimize: true},
		"default/availability": {Query: "", ProfileType: common.ProfileTypeFromText("availability"), Minimize: false},
	}
	state := getCurrentState(cfg, client, informer, objective, errors, profiles)
	if state.CurrentPods["my-deployment-0"].Availability != state.Intent.Objectives["default/availability"] || state.Intent.Objectives["availability"] == 1.0 {
		t.Errorf("Availability should be set and below 1.0 - was %f", state.Intent.Objectives["default/availability"])
	}
	if state.Intent.Objectives["default/p99latency"] != 10.0 {
		t.Errorf("P99 should have been 10.0 - was %f.", state.Intent.Objectives["default/p99latency"])
	}
	if state.CurrentData["bla"]["node0"] != 10.0 {
		t.Errorf("Host data should have been 10.0 - was %f.", state.CurrentData["bla"]["node0"])
	}
	if len(state.Resources) != 4 {
		t.Errorf("Should see resources requests/limits for both containers - found %v", state.Resources)
	}
}

// TestGetDesiredStateForSanity test for sanity.
func TestGetDesiredStateForSanity(t *testing.T) {
	objective := common.Intent{
		Objectives: map[string]float64{"foo": 0.1},
	}
	res := getDesiredState(objective)
	if res.Intent.Objectives["foo"] != objective.Objectives["foo"] {
		t.Errorf("This should be equal: %v - %v.", res.Intent.Objectives, objective.Objectives)
	}
}
