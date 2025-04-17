package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/klog/v2"

	"github.com/intel/intent-driven-orchestration/pkg/api/intents/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/controller"
	"github.com/intel/intent-driven-orchestration/pkg/generated/clientset/versioned/fake"
	informers "github.com/intel/intent-driven-orchestration/pkg/generated/informers/externalversions"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators/scaling"
	"github.com/intel/intent-driven-orchestration/pkg/planner/astar"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	k8sInformers "k8s.io/client-go/informers"
	k8sFake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

// tries defines the number of times we'll try to add objects.
const tries = 60

// timeoutInMillis defines the timout between retries.
const timeoutInMillis = 750

type planEvent struct {
	plan []planner.Action
	name string
}

// fileTracer is a simplified version of the knowledge base.
type fileTracer struct {
	ch           chan planEvent
	data         map[string]map[string]map[string][]interface{}
	indexer      map[string]map[string]map[string]int
	indexerMutex *sync.RWMutex
}

func (t fileTracer) TraceEvent(_ common.State, desired common.State, plan []planner.Action) {
	t.ch <- planEvent{
		plan: plan,
		name: desired.Intent.Key,
	}
}

func (t fileTracer) GetEffect(name string, group string, profileName string, _ int, constructor func() interface{}) (interface{}, error) {
	if _, ok := t.indexer[group]; !ok {
		return nil, fmt.Errorf("group not found in dataset: %v", group)
	}
	t.indexerMutex.RLock()
	index := t.indexer[group][profileName][name]
	t.indexerMutex.RUnlock()

	data := t.data[group][profileName][name][index].(map[string]interface{})
	if group == "scaling" {
		tmp := constructor().(*scaling.ScaleOutEffect)
		popt := [4]float64{}
		for i, v := range data["popt"].([]interface{}) {
			popt[i] = v.(float64)
		}
		tmp.Popt = popt
		replicaRange := [2]int{}
		for i, v := range data["replicaRange"].([]interface{}) {
			replicaRange[i] = int(v.(float64))
		}
		tmp.ReplicaRange = replicaRange
		throughputScale := [2]float64{}
		for i, v := range data["throughputScale"].([]interface{}) {
			throughputScale[i] = v.(float64)
		}
		tmp.ThroughputScale = throughputScale
		return tmp, nil
	} else if group == "vertical_scaling" {
		tmp := constructor().(*scaling.CPUScaleEffect)
		popt := [3]float64{}
		for i, v := range data["popt"].([]interface{}) {
			popt[i] = v.(float64)
		}
		tmp.Popt = popt
		return tmp, nil
	}
	return nil, nil
}

// stepIndex increments the index, so we can retrieve updated models.
func (t fileTracer) stepIndex(index int) {
	t.indexerMutex.Lock()
	defer t.indexerMutex.Unlock()
	for k1, v1 := range t.indexer {
		for k2, v2 := range v1 {
			for k3 := range v2 {
				if index < len(t.data[k1][k2][k3]) {
					t.indexer[k1][k2][k3] = index
				} else {
					klog.Infof("Cannot find another entry for %v-%v-%v will use last!", k1, k2, k3)
				}
			}
		}
	}
}

// actuatorSetup contains information about the way to initialize and configure actuators.
type actuatorSetup struct {
	initFunc interface{}
	cfg      interface{}
}

// testEnvironment holds all info needed for the replay.
type testEnvironment struct {
	name            string
	effectsFilename string
	eventsFilename  string
	defaults        *common.Config
	actuators       map[string]actuatorSetup
}

// testFixture for the replay test.
type testFixture struct {
	test             *testing.T
	objects          []runtime.Object
	k8sClient        *k8sFake.Clientset
	intentClient     *fake.Clientset
	k8sInformer      k8sInformers.SharedInformerFactory
	intentInformer   informers.SharedInformerFactory
	tracer           fileTracer
	prometheus       prometheusDummy
	prometheusServer *http.Server
	ticker           chan planEvent
}

// newTestFixture creates a new fixture for testing.
func newTestFixture(test *testing.T) testFixture {
	f := testFixture{}
	f.test = test
	return f
}

// prometheusDummy enables serving values to the framework.
type prometheusDummy struct {
	vals    map[string]float64
	valLock *sync.RWMutex
}

// prometheusValues holds the values the prometheus dummy can return.
type prometheusValues struct {
	vals map[string]float64
}

// updateValues enables the values that are going to be returned to the framework.
func (p *prometheusDummy) updateValues() chan<- prometheusValues {
	updates := make(chan prometheusValues)
	go func() {
		for e := range updates {
			p.valLock.Lock()
			for key, val := range e.vals {
				p.vals[key] = val
			}
			p.valLock.Unlock()
		}
	}()
	return updates
}

// serve handles the HTTP requests - mimics a prometheus server.
func (p *prometheusDummy) serve() *http.Server {
	mux := http.NewServeMux()
	// return KPI related information.
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		p.valLock.Lock()
		key := strings.Split(r.URL.Query().Get("query"), "&")[0]
		_, err := fmt.Fprintf(w, "{\"data\": {\"result\": [{\"value\": [1680347032.000, \"%f\"]}]}}", p.vals[key])
		if err != nil {
			return
		}
		p.valLock.Unlock()
	})
	// returns host based telemetry information - assumes query contains a host and metrics name seperated by @.
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		p.valLock.Lock()
		key := strings.Split(r.URL.Query().Get("query"), "&")[0]
		host := strings.Split(key, "@")[1]
		_, err := fmt.Fprintf(w, "{\"data\": {\"result\": [{\"metric\": {\"host\": \"%v\"}, \"value\": [1680347032.000, \"%f\"]}]}}", host, p.vals[key])
		if err != nil {
			return
		}
		p.valLock.Unlock()
	})

	server := &http.Server{
		Addr:              ":39090",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			if strings.Contains(err.Error(), "address already in use") {
				klog.Errorf("Assigned Port address already in use in here, skipping test: %v", err)
			} else {
				klog.Fatalf("Could not serve: %v", err)
			}

		}
	}()
	return server
}

// checkPrometheus makes sure the dummy prometheus server can be reached.
func (f *testFixture) checkPrometheus() {
	ready := false
	for i := 0; i < tries; i++ {
		time.Sleep(time.Millisecond * timeoutInMillis)
		resp, err := http.Get("http://127.0.0.1:39090/query")
		if err != nil || resp.Status != "200 OK" {
			klog.Warningf("Could not reach prometheus: %v - %v.", err, resp.Status)
		} else {
			ready = true
			break
		}
	}
	if !ready {
		f.test.Errorf("Failed to reach prometheus dummy web server!")
	}
}

// checkProfiles makes sure that all KPI profiles have been added.
func (f *testFixture) checkProfiles(profiles map[string]float64) {
	// now let's wait till we've seen the status updates to the KPI profiles...
	counter := 0
	ready := false
	for i := 0; i < tries; i++ {
		for _, action := range f.intentClient.Actions() {
			if action.GetSubresource() == "status" && action.GetVerb() == "update" {
				counter++
			}
			if counter == len(profiles) {
				ready = true
			}
		}
		if ready {
			break
		}
		time.Sleep(time.Millisecond * timeoutInMillis)
	}
	if !ready {
		f.test.Errorf("Profiles were not added in time: %v.", profiles)
	}
}

// newTestSetup sets up a new version of the test.
func (f *testFixture) newTestSetup(env testEnvironment, stopper chan struct{}) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	// load trace.
	effects, indexer, err := parseEffects(env.effectsFilename)
	if err != nil {
		f.test.Errorf("Could not load trace: %s.", err)
	}

	// fake environment...
	f.ticker = make(chan planEvent)
	f.tracer = fileTracer{ch: f.ticker, data: effects, indexer: indexer, indexerMutex: &sync.RWMutex{}}
	f.prometheus = prometheusDummy{valLock: &sync.RWMutex{}, vals: make(map[string]float64)}
	f.k8sClient = k8sFake.NewSimpleClientset(f.objects...)
	f.intentClient = fake.NewSimpleClientset(f.objects...)
	fakeWatch := watch.NewFake()
	f.intentClient.PrependWatchReactor("KpiProfile", core.DefaultWatchReactor(fakeWatch, nil))
	f.intentClient.PrependWatchReactor("Intents", core.DefaultWatchReactor(fakeWatch, nil))

	// informers
	f.k8sInformer = k8sInformers.NewSharedInformerFactory(f.k8sClient, func() time.Duration { return 0 }())
	f.intentInformer = informers.NewSharedInformerFactory(f.intentClient, func() time.Duration { return 0 }())

	// we provide the actuators with a dummy client that does nothing; so the framework test can make sure for each cycle the right state is setup.
	dummyK8sClient := k8sFake.NewSimpleClientset(f.objects...)

	// plnr and actuator setup
	var actuatorList []actuators.Actuator
	for _, actuatorConfig := range env.actuators {
		in := make([]reflect.Value, 3)
		in[0] = reflect.ValueOf(dummyK8sClient)
		in[1] = reflect.ValueOf(f.tracer)
		in[2] = reflect.ValueOf(actuatorConfig.cfg)

		var res []reflect.Value
		f := reflect.ValueOf(actuatorConfig.initFunc)
		res = f.Call(in)
		result := res[0].Interface()
		actuatorList = append(actuatorList, result.(actuators.Actuator))
	}
	plnr := astar.NewAPlanner(actuatorList, *env.defaults)
	defer plnr.Stop()

	// intent controller...
	ctlr := controller.NewController(*env.defaults, f.tracer, f.k8sClient, f.k8sInformer.Core().V1().Pods())
	ctlr.SetPlanner(plnr)
	go ctlr.Run(1, stopper)

	// profile monitor...
	profileMonitor := controller.NewKPIProfileMonitor(env.defaults.Monitor, f.intentClient, f.intentInformer.Ido().V1alpha1().KPIProfiles(), ctlr.UpdateProfile())
	go profileMonitor.Run(1, stopper)

	// intent monitor...
	intentMonitor := controller.NewIntentMonitor(f.intentClient, f.intentInformer.Ido().V1alpha1().Intents(), ctlr.UpdateIntent())
	go intentMonitor.Run(1, stopper)

	// pod monitor...
	podMonitor := controller.NewPodMonitor(f.k8sClient, f.k8sInformer.Core().V1().Pods(), ctlr.UpdatePodError())
	go podMonitor.Run(1, stopper)

	// start a prometheus dummy and run the framework
	f.prometheusServer = f.prometheus.serve()

	f.k8sInformer.Start(ctx.Done())
	f.intentInformer.Start(ctx.Done())

	// let's give all workers a chance to spin up...
	time.Sleep(time.Millisecond * timeoutInMillis)

	return cancel
}

// Event represents an entry from an events' collection.
type Event struct {
	Intent    string                            `json:"name"`
	Current   map[string]float64                `json:"current_objectives"`
	Desired   map[string]float64                `json:"desired_objectives"`
	Pods      map[string]map[string]interface{} `json:"pods"`
	Resources map[string]int64                  `json:"resources"`
	Plan      []map[string]interface{}          `json:"plan"`
	Data      map[string]map[string]float64     `json:"data"`
}

// Effect represents an entry in an effects' collection.
type Effect struct {
	Name        string                 `json:"name"`
	ProfileName string                 `json:"profileName"`
	Group       string                 `json:"group"`
	Data        map[string]interface{} `json:"data"`
}

// parseTrace reads a trace from a json file.
func parseTrace(filename string) []Event {
	trace, err := os.Open(filename)
	if err != nil {
		klog.Errorf("Could not open events trace: %v.", err)
	}
	defer func(trace *os.File) {
		err := trace.Close()
		if err != nil {
			klog.Errorf("Now this should not happen: %v.", err)
		}
	}(trace)

	var events []Event
	tmp, err1 := io.ReadAll(trace)
	err2 := json.Unmarshal(tmp, &events)
	if err1 != nil || err2 != nil {
		klog.Errorf("Could not read and/or unmarshal trace: %v-%v.", err1, err2)
	}
	return events
}

// parseEffects reads a set of effects from a json file.
func parseEffects(filename string) (map[string]map[string]map[string][]interface{}, map[string]map[string]map[string]int, error) {
	effects, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open effects effects: %v", err)
	}
	defer func(trace *os.File) {
		err := trace.Close()
		if err != nil {
			klog.Errorf("Now this should not happen: %v.", err)
		}
	}(effects)

	var events []Effect
	tmp, err1 := io.ReadAll(effects)
	err2 := json.Unmarshal(tmp, &events)
	if err1 != nil || err2 != nil {
		return nil, nil, fmt.Errorf("could not read and/or unmarshal effects trace: %v-%v", err1, err2)
	}

	data := make(map[string]map[string]map[string][]interface{})
	indexes := make(map[string]map[string]map[string]int)
	for _, effect := range events {
		if _, ok := data[effect.Group]; !ok {
			data[effect.Group] = make(map[string]map[string][]interface{})
			indexes[effect.Group] = make(map[string]map[string]int)
		}
		if _, ok := data[effect.Group][effect.ProfileName]; !ok {
			data[effect.Group][effect.ProfileName] = make(map[string][]interface{})
			indexes[effect.Group][effect.ProfileName] = make(map[string]int)
		}
		indexes[effect.Group][effect.ProfileName][effect.Name] = 0
		data[effect.Group][effect.ProfileName][effect.Name] = append(data[effect.Group][effect.ProfileName][effect.Name], effect.Data)
	}

	return data, indexes, nil
}

// setupProfiles initially defines the profiles.
func (f *testFixture) setupProfiles(profiles map[string]float64) {
	// FIXME: we should store all information related to the profiles ano not use names to infer types.
	for name := range profiles {
		tmp := strings.Split(name, "/")
		typeName := "throughput"
		if strings.Contains(tmp[1], "latency") {
			typeName = "latency"
		}
		profile := &v1alpha1.KPIProfile{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      tmp[1],
				Namespace: tmp[0],
			},
			Spec: v1alpha1.KPIProfileSpec{
				KPIType: typeName,
			},
		}
		_, err := f.intentClient.IdoV1alpha1().KPIProfiles(tmp[0]).Create(context.TODO(), profile, metaV1.CreateOptions{})
		if err != nil {
			f.test.Errorf("Could not add profile: %v.", err)
		}
		klog.Infof("Adding profile: %s.", name) // For some weird reason this logging is important for timing reasons.
	}
	f.checkProfiles(profiles)
}

// setWorkloadState updates the deployment and pod specs.
func (f *testFixture) setWorkloadState(pods map[string]map[string]interface{}, resources map[string]int64) {
	// FIXME: current we do not store information on the workload - we could pick it up from a manifest later on.
	// if deployment does not exist - add it.
	res, err := f.k8sClient.AppsV1().Deployments("default").Get(context.TODO(), "function-deployment", metaV1.GetOptions{})
	if err != nil || res == nil {
		repl := int32(len(pods)) //nolint:gosec // explanation: casting len to int64 for API compatibility
		deployment := &appsV1.Deployment{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "function-deployment",
				Namespace: metaV1.NamespaceDefault,
			},
			Spec: appsV1.DeploymentSpec{
				Replicas: &repl,
				Selector: &metaV1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "sample-function",
					},
				},
				Template: coreV1.PodTemplateSpec{
					Spec: coreV1.PodSpec{
						Containers: []coreV1.Container{
							{
								Name: "function",
								Resources: coreV1.ResourceRequirements{
									Limits:   make(map[coreV1.ResourceName]resource.Quantity),
									Requests: make(map[coreV1.ResourceName]resource.Quantity),
								},
							},
						},
					},
				},
			},
		}
		_, err := f.k8sClient.AppsV1().Deployments("default").Create(context.TODO(), deployment, metaV1.CreateOptions{})
		if err != nil {
			klog.Errorf("Could not add deployment: %v", err)
		}
	}
	// add all pods from trace.
	for key := range pods {
		res, err := f.k8sClient.CoreV1().Pods("default").Get(context.TODO(), key, metaV1.GetOptions{})
		if err != nil || res == nil {
			pod := &coreV1.Pod{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      key,
					Labels:    map[string]string{"app": "sample-function"},
					Namespace: "default",
				},
				Status: coreV1.PodStatus{
					Phase:    coreV1.PodPhase(pods[key]["state"].(string)),
					QOSClass: coreV1.PodQOSClass(pods[key]["qosclass"].(string)),
				},
				Spec: coreV1.PodSpec{
					NodeName: pods[key]["nodename"].(string),
				},
			}
			var containers []coreV1.Container
			keys := make([]string, 0)
			for k := range resources {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				val := resources[key]
				tmp := strings.Split(key, "_")
				index, err := strconv.Atoi(tmp[0])
				if err != nil {
					klog.Infof("Error in splitting: %s.", err)
				}
				if index > len(containers)-1 {
					containers = append(containers, coreV1.Container{
						Name: tmp[0],
						Resources: coreV1.ResourceRequirements{
							Limits:   make(map[coreV1.ResourceName]resource.Quantity),
							Requests: make(map[coreV1.ResourceName]resource.Quantity),
						},
					})
				}
				if tmp[2] == "limits" {
					quan := resource.NewMilliQuantity(val, resource.DecimalSI)
					containers[index].Resources.Limits[coreV1.ResourceName(tmp[1])] = *quan
				} else if tmp[2] == "requests" {
					quan := resource.NewMilliQuantity(val, resource.DecimalSI)
					containers[index].Resources.Requests[coreV1.ResourceName(tmp[1])] = *quan
				}
			}
			pod.Spec.Containers = containers
			_, err := f.k8sClient.CoreV1().Pods("default").Create(context.TODO(), pod, metaV1.CreateOptions{})
			if err != nil {
				klog.Errorf("Could not add pod: %v", err)
			}
		}
	}
	// remove all pods that should no longer exists.
	activePods, err := f.k8sClient.CoreV1().Pods("default").List(context.TODO(), metaV1.ListOptions{})
	if err != nil {
		klog.Errorf("Now this should never happen: %v.", err)
	}
	for _, active := range activePods.Items {
		_, ok := pods[active.Name]
		if !ok {
			err := f.k8sClient.CoreV1().Pods("default").Delete(context.TODO(), active.Name, metaV1.DeleteOptions{})
			if err != nil {
				klog.Errorf("Now this should never happen: %v.", err)
			}
		}
	}
}

// setupIntent defines the initial intent.
func (f *testFixture) setupIntent(name string, objectives map[string]float64) {
	tmp := strings.Split(name, "/")
	myIntent := &v1alpha1.Intent{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      tmp[1],
			Namespace: tmp[0],
		},
		Spec: v1alpha1.IntentSpec{
			Priority: 1.0,
			TargetRef: v1alpha1.TargetRef(struct {
				Kind string
				Name string
			}{Kind: "Deployment", Name: "default/function-deployment"}),
		},
	}
	for key, val := range objectives {
		tmp := v1alpha1.TargetObjective{
			Name:       key,
			MeasuredBy: key,
			Value:      val,
		}
		myIntent.Spec.Objectives = append(myIntent.Spec.Objectives, tmp)
	}
	_, err := f.intentClient.IdoV1alpha1().Intents(tmp[0]).Create(context.TODO(), myIntent, metaV1.CreateOptions{})
	if err != nil {
		klog.Errorf("Could not add intent: %v.", err)
	}
}

// setDesiredObjectives updates an existing intent.
func (f *testFixture) setDesiredObjectives(name string, objectives map[string]float64) {
	tmp := strings.Split(name, "/")
	myIntent, err := f.intentClient.IdoV1alpha1().Intents(tmp[0]).Get(context.TODO(), tmp[1], metaV1.GetOptions{})
	if err != nil {
		klog.Errorf("Could not retrieve previous set intent: %v.", err)
	}
	changed := false
	myIntent = myIntent.DeepCopy()
	myIntent.ResourceVersion += "1"
	for i, objv := range myIntent.Spec.Objectives {
		if objectives[objv.Name] != objv.Value {
			myIntent.Spec.Objectives[i].Value = objectives[objv.Name]
			changed = true
		}
	}
	if changed {
		_, err = f.intentClient.IdoV1alpha1().Intents(tmp[0]).Update(context.TODO(), myIntent, metaV1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Could not update intent: %v.", err)
		}
	} else {
		klog.Infof("No change in intent - will not update: %v", objectives)
	}
}

// setCurrentObjectives updates the values that the prometheus dummy will report back.
func (f *testFixture) setCurrentObjectives(values map[string]float64) {
	f.prometheus.updateValues() <- prometheusValues{vals: values}
}

// setCurrentData updates the values that the prometheus dummy will report back for host related metrics.
func (f *testFixture) setCurrentData(vals map[string]map[string]float64) {
	data := make(map[string]float64)
	for metric, entry := range vals {
		for host, val := range entry {
			data[metric+"@"+host] = val
		}
	}
	f.prometheus.updateValues() <- prometheusValues{vals: data}
}

// comparePlans compares two planes and returns false if they are not the same.
func comparePlans(onePlan []map[string]interface{}, anotherPlan []planner.Action) bool {
	if len(anotherPlan) != len(onePlan) {
		return false
	}
	var oldPlan []planner.Action
	for _, entry := range onePlan {
		tmp := planner.Action{
			Name:       entry["name"].(string),
			Properties: entry["properties"],
		}
		oldPlan = append(oldPlan, tmp)
	}
	for i, item := range oldPlan {
		if item.Name != anotherPlan[i].Name {
			klog.Infof("Expected action name: %v - got %v", item.Name, anotherPlan[i].Name)
			return false
		}
		one := fmt.Sprintf("%v", item.Properties)
		another := fmt.Sprintf("%v", anotherPlan[i].Properties)
		if one != another && item.Name != "rmPod" {
			klog.Infof("Expected property: %v - got %v", one, another)
			return false
		} else if item.Name == "rmPod" {
			if one != another && len(one) == len(another) {
				klog.Warningf("Not super sure - but looks ok: %v - %v", item.Properties, anotherPlan[i].Properties)
			} else if one != another {
				klog.Infof("This does not look right; expected: %v - got %v", one, another)
				return false
			}
		}
	}
	return true
}

// runTrace tries to retrace a single trace.
func runTrace(env testEnvironment, t *testing.T) {
	f := newTestFixture(t)
	stopChannel := make(chan struct{})
	defer close(stopChannel)

	cancel := f.newTestSetup(env, stopChannel)

	events := parseTrace(env.eventsFilename)
	// We'll use the first entry in the trace to set up the system.
	f.setupProfiles(events[0].Desired)
	f.setupIntent(events[0].Intent, events[0].Desired)
	f.setCurrentObjectives(events[0].Current)
	f.setCurrentData(events[0].Data)
	f.setWorkloadState(events[0].Pods, events[0].Resources)
	f.checkPrometheus()
	<-f.ticker // although we use first entry for setup, we should wait for first plan; but don't need to compare.

	// now replay rest of the trace.
	for i := 1; i < len(events); i++ {
		fmt.Println(strconv.Itoa(i) + "----")
		f.setCurrentObjectives(events[i].Current)
		f.setCurrentData(events[i].Data)
		f.setWorkloadState(events[i].Pods, events[i].Resources)
		f.setDesiredObjectives(events[i].Intent, events[i].Desired)
		f.tracer.stepIndex(i)

		planEvent := <-f.ticker
		if !comparePlans(events[i].Plan, planEvent.plan) {
			t.Errorf("Expected %v - got %v.", events[i].Plan, planEvent.plan)
		}
	}

	err := f.prometheusServer.Shutdown(context.TODO())
	if err != nil {
		klog.Errorf("Error while shutdown of prometheus server: %v", err)
	}
	stopChannel <- struct{}{}
	cancel()
}

// TestTracesForSanity checks if various set of traces work.
func TestTracesForSanity(t *testing.T) {
	defaultsConfig, err1 := common.LoadConfig("traces/defaults.json", func() interface{} {
		return &common.Config{}
	})
	scaleOutConfig, err2 := common.LoadConfig("traces/scale_out.json", func() interface{} {
		return &scaling.ScaleOutConfig{}
	})
	rmPodConfig, err3 := common.LoadConfig("traces/rm_pod.json", func() interface{} {
		return &scaling.RmPodConfig{}
	})
	cpuScaleConfig, err4 := common.LoadConfig("traces/cpu_scale.json", func() interface{} {
		return &scaling.CPUScaleConfig{}
	})
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		t.Errorf("Could not load config files!")
	}

	var tests = []testEnvironment{
		{name: "fabricated_trace", effectsFilename: "traces/trace_0/effects.json", eventsFilename: "traces/trace_0/events.json", defaults: defaultsConfig.(*common.Config), actuators: map[string]actuatorSetup{
			"NewCPUScaleActuator": {scaling.NewCPUScaleActuator, *cpuScaleConfig.(*scaling.CPUScaleConfig)}},
		},
		{name: "horizontal_vertical_scaling", effectsFilename: "traces/trace_1/effects.json", eventsFilename: "traces/trace_1/events.json", defaults: defaultsConfig.(*common.Config), actuators: map[string]actuatorSetup{
			"NewCPUScaleActuator": {scaling.NewCPUScaleActuator, *cpuScaleConfig.(*scaling.CPUScaleConfig)},
			"NewRmPodActuator":    {scaling.NewRmPodActuator, *rmPodConfig.(*scaling.RmPodConfig)},
			"NewScaleOutActuator": {scaling.NewScaleOutActuator, *scaleOutConfig.(*scaling.ScaleOutConfig)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTrace(tt, t)
		})
	}
}
