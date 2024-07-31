package controller

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
)

// profileUpdateWaitGroup is a silly way to make sure we don't have race conditions in our test code.
var podUpdateWaitGroup sync.WaitGroup

// podFixture basic test struct.
type podFixture struct {
	test            *testing.T
	client          *fake.Clientset
	podLister       []*coreV1.Pod
	objects         []runtime.Object
	actualUpdates   []common.PodError
	expectedUpdates []common.PodError
}

// newPodFixture initializes the test struct.
func newPodFixture(test *testing.T) *podFixture {
	f := &podFixture{}
	f.test = test
	return f
}

// acceptUpdates channel function to accept profile actualUpdates.
func (f *podFixture) acceptUpdates() chan<- common.PodError {
	events := make(chan common.PodError)
	go func() {
		for {
			e := <-events
			podUpdateWaitGroup.Add(1)
			f.actualUpdates = append(f.actualUpdates, e)
			podUpdateWaitGroup.Done()
		}
	}()
	return events
}

// newMonitor creates a new monitor tailored for testing.
func (f *podFixture) newMonitor(stop chan struct{}) (*PodMonitor, *watch.FakeWatcher) {
	// new client
	f.client = fake.NewSimpleClientset(f.objects...)
	fakeWatch := watch.NewFake()
	f.client.PrependWatchReactor("pods", core.DefaultWatchReactor(fakeWatch, nil))

	// new pod monitor
	informer := informers.NewSharedInformerFactory(f.client, func() time.Duration { return 0 }())
	mon := NewPodMonitor(f.client, informer.Core().V1().Pods(), f.acceptUpdates())
	mon.podSynced = func() bool { return true }

	for _, f := range f.podLister {
		err := informer.Core().V1().Pods().Informer().GetIndexer().Add(f)
		if err != nil {
			klog.Fatal(err)
		}
	}

	informer.Start(stop)
	return mon, fakeWatch
}

// testSyncHandler tests if the expected updates etc. get called.
func (f *podFixture) testSyncHandler(key string, mon *PodMonitor) {
	err := mon.syncHandler(key)
	if err != nil {
		f.test.Error(fmt.Errorf("should not throw an error: %s", err))
	}

	// make sure the updates are always captured.
	time.Sleep(timeout * time.Millisecond)
	podUpdateWaitGroup.Wait()

	// check if update came through.
	for i, actualUpdate := range f.actualUpdates {
		if actualUpdate.Start != f.expectedUpdates[i].Start || actualUpdate.End != f.expectedUpdates[i].End {
			f.test.Errorf("Update mismatch: %v - %v.", actualUpdate, f.expectedUpdates[i])
			break
		}
	}
	if len(f.actualUpdates) != len(f.expectedUpdates) {
		f.test.Errorf("Number of actual and expected update(s) mismatched: %d - %d.", len(f.actualUpdates), len(f.expectedUpdates))
	}
}

// newPod creates a dummy Pod.
func newPod(name string) *coreV1.Pod {
	return &coreV1.Pod{
		TypeMeta: metaV1.TypeMeta{APIVersion: coreV1.SchemeGroupVersion.String()},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Labels:    map[string]string{"foo": "bar"},
			Namespace: metaV1.NamespaceDefault,
		},
		Spec: coreV1.PodSpec{
			Containers: []coreV1.Container{},
		},
	}
}

// Tests for success.

// TestRunPodMonitorForSuccess tests for success.
func TestRunPodMonitorForSuccess(t *testing.T) {
	f := newPodFixture(t)

	dummyPod := newPod("foo")
	f.podLister = append(f.podLister, dummyPod)
	f.objects = append(f.objects, dummyPod)

	stop := make(chan struct{})
	defer close(stop)
	mon, faker := f.newMonitor(stop)

	// syncHandler "replaced" by sth simpler...
	mon.syncHandler = func(_ string) error {
		return nil
	}

	go mon.Run(1, stop)
	modifiedPod := dummyPod.DeepCopy()
	modifiedPod.ResourceVersion = "foobar"
	faker.Modify(modifiedPod)

	time.Sleep(timeout * time.Millisecond)
}

// TestProcessPodForSuccess tests for success.
func TestProcessPodForSuccess(t *testing.T) {
	f := newPodFixture(t)

	dummyPod := newPod("foo")
	f.podLister = append(f.podLister, dummyPod)
	f.objects = append(f.objects, dummyPod)

	stop := make(chan struct{})
	defer close(stop)
	mon, faker := f.newMonitor(stop)

	f.testSyncHandler("default/foo", mon)
	faker.Modify(dummyPod)
}

// Tests for failure.

// TestRunPodMonitorForFailure tests for failure.
func TestRunPodMonitorForFailure(t *testing.T) {
	f := newPodFixture(t)
	stop := make(chan struct{})
	defer close(stop)

	pod := newPod("foo")
	f.podLister = append(f.podLister, pod)
	f.objects = append(f.objects, pod)

	// syncHandler will raise an error.
	mon, faker := f.newMonitor(stop)
	mon.syncHandler = func(_ string) error {
		return errors.New("oops")
	}

	// TODO: assert that pod got added back to queue.
	go mon.Run(1, stop)
	modifiedPod := pod.DeepCopy()
	modifiedPod.ResourceVersion = "abc"
	faker.Modify(modifiedPod)

	time.Sleep(timeout * time.Millisecond)
}

// TestProcessPodForFailure tests for failure.
func TestProcessPodForFailure(t *testing.T) {
	f := newPodFixture(t)

	stop := make(chan struct{})
	defer close(stop)
	mon, _ := f.newMonitor(stop)

	// invalid key.
	f.testSyncHandler("foo/bar/123", mon)

	// non-existing entry.
	f.testSyncHandler("default/foo", mon)
}

// Tests for sanity.

// TestRunPodMonitorForSanity tests for sanity.
func TestRunPodMonitorForSanity(t *testing.T) {
	f := newPodFixture(t)

	dummyPod := newPod("flaky-pod")

	stop := make(chan struct{})
	defer close(stop)
	mon, faker := f.newMonitor(stop)

	// syncHandler "replaced" by sth simpler...
	mon.syncHandler = func(_ string) error {
		return nil
	}

	go mon.Run(1, stop)
	updatedPod := dummyPod.DeepCopy()
	updatedPod.ResourceVersion = "abc"
	faker.Modify(updatedPod)
	faker.Delete(dummyPod)

	// assert we got an update call.
	time.Sleep(timeout * time.Millisecond)
	podUpdateWaitGroup.Wait()
	if !f.actualUpdates[0].Start.IsZero() {
		t.Error(fmt.Errorf("expected an update with zero time to remove pod from tracked podErrors"))
	}
}

// TestProcessPodForSanity tests for sanity.
func TestProcessPodForSanity(t *testing.T) {
	f := newPodFixture(t)

	// time range for an error taking 1 min.
	start := metaV1.NewTime(time.Date(2022, 2, 24, 10, 0, 0, 0, time.UTC))
	end := metaV1.NewTime(time.Date(2022, 2, 24, 10, 1, 0, 0, time.UTC))

	dummyPod := newPod("flaky-pod")
	term := coreV1.ContainerStateTerminated{FinishedAt: start}
	run := coreV1.ContainerStateRunning{StartedAt: end}
	lastState := coreV1.ContainerState{Terminated: &term}
	currState := coreV1.ContainerState{Running: &run}
	state := coreV1.ContainerStatus{Ready: false, LastTerminationState: lastState, State: currState}
	dummyPod.Status.ContainerStatuses = []coreV1.ContainerStatus{state}
	f.podLister = append(f.podLister, dummyPod)
	f.objects = append(f.objects, dummyPod)

	stop := make(chan struct{})
	defer close(stop)
	mon, _ := f.newMonitor(stop)

	// First time around we notice an error...
	f.testSyncHandler("default/flaky-pod", mon)
	mon.cacheLock.Lock()
	if _, ok := mon.podsWithError["default/flaky-pod"]; !ok {
		t.Errorf("POD should be in the map.")
	}
	mon.cacheLock.Unlock()

	// ...and once it is ready again we track the time.
	f.expectedUpdates = append(f.expectedUpdates, common.PodError{Key: "default/flaky-pod", Start: start.Time, End: end.Time})
	pod, _ := mon.podLister.Pods("default").Get("flaky-pod")
	pod.Status.ContainerStatuses[0].Ready = true
	f.testSyncHandler("default/flaky-pod", mon)
	mon.cacheLock.Lock()
	if _, ok := mon.podsWithError["default/flaky-pod"]; ok {
		t.Errorf("POD should not be in the map")
	}
	mon.cacheLock.Unlock()
}
