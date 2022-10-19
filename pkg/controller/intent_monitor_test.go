package controller

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/api/intents/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/generated/clientset/versioned/fake"
	informers "github.com/intel/intent-driven-orchestration/pkg/generated/informers/externalversions"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
)

// intentUpdateWaitGroup is a silly way to make sure we don't have race conditions in our test code.
var intentUpdateWaitGroup sync.WaitGroup

// intentFixture represents a basic test case.
type intentFixture struct {
	test            *testing.T
	client          *fake.Clientset
	intentLister    []*v1alpha1.Intent
	objects         []runtime.Object
	expectedUpdates []common.Intent
	actualUpdates   []common.Intent
}

// newIntentFixture initializes the test.
func newIntentFixture(test *testing.T) *intentFixture {
	f := &intentFixture{}
	f.test = test
	return f
}

// acceptUpdates channel function to accept intent updates.
func (f *intentFixture) acceptUpdates() chan<- common.Intent {
	events := make(chan common.Intent)
	go func() {
		for {
			e := <-events
			intentUpdateWaitGroup.Add(1)
			f.actualUpdates = append(f.actualUpdates, e)
			intentUpdateWaitGroup.Done()
		}
	}()
	return events
}

// newMonitor creates a new monitor tailored for testing.
func (f *intentFixture) newMonitor(done chan struct{}) (*IntentMonitor, *watch.FakeWatcher) {
	f.client = fake.NewSimpleClientset(f.objects...)
	faker := watch.NewFake()
	f.client.PrependWatchReactor("intents", core.DefaultWatchReactor(faker, nil))

	// new intent monitor
	informer := informers.NewSharedInformerFactory(f.client, func() time.Duration { return 0 }())
	mon := NewIntentMonitor(f.client, informer.Ido().V1alpha1().Intents(), f.acceptUpdates())
	mon.intentSynced = func() bool { return true }

	for _, f := range f.intentLister {
		err := informer.Ido().V1alpha1().Intents().Informer().GetIndexer().Add(f)
		if err != nil {
			klog.Fatal(err)
		}
	}

	informer.Start(done)
	return mon, faker
}

// testSyncHandler processes and objects and checks if right actions have been performed.
func (f *intentFixture) testSyncHandler(key string) {
	done := make(chan struct{})
	defer close(done)
	mon, _ := f.newMonitor(done)

	// should not throw an error.
	err := mon.syncHandler(key)
	if err != nil {
		f.test.Error(fmt.Errorf("error running test: %s", err))
	}

	// make sure the updates are always captured.
	time.Sleep(timeout * time.Millisecond)
	intentUpdateWaitGroup.Wait()

	// check if updates came through.
	for i, actualUpdate := range f.actualUpdates {
		if actualUpdate.Key != f.expectedUpdates[i].Key || actualUpdate.Priority != f.expectedUpdates[i].Priority {
			f.test.Errorf("Update mismatch: %v - %v.", actualUpdate, f.expectedUpdates[i])
			break
		}
	}
	if len(f.actualUpdates) != len(f.expectedUpdates) {
		f.test.Errorf("Number of actual and expected update(s) mismatched: %d - %d.", len(f.actualUpdates), len(f.expectedUpdates))
	}

	// cleanup
	f.actualUpdates = make([]common.Intent, 0)
	f.expectedUpdates = make([]common.Intent, 0)
}

// newIntent creates an intent for testing purposes.
func newIntent(name string, p99 float64) *v1alpha1.Intent {
	return &v1alpha1.Intent{
		TypeMeta: metaV1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: metaV1.NamespaceDefault,
		},
		Spec: v1alpha1.IntentSpec{
			Priority: 0.0,
			TargetRef: v1alpha1.TargetRef{
				Name: "my-deployment",
				Kind: "Deployment",
			},
			Objectives: []v1alpha1.TargetObjective{
				{
					Name:       "P99",
					Value:      p99,
					MeasuredBy: "p99latency",
				},
			},
		},
	}
}

// Tests for success.

// TestRunIntentMonitorForSuccess tests for success.
func TestRunIntentMonitorForSuccess(t *testing.T) {
	f := newIntentFixture(t)
	intent := newIntent("foo", 100)

	stopper := make(chan struct{})
	defer close(stopper)

	mon, faker := f.newMonitor(stopper)
	mon.syncHandler = func(key string) error {
		return nil
	}

	go mon.Run(1, stopper)
	faker.Add(intent)
}

// TestProcessIntentForSuccess tests for success.
func TestProcessIntentForSuccess(t *testing.T) {
	f := newIntentFixture(t)
	intent := newIntent("bar", 100)
	f.objects = append(f.objects, intent)
	f.intentLister = append(f.intentLister, intent)
	f.expectedUpdates = []common.Intent{{Key: "default/bar"}}
	f.testSyncHandler("default/bar")
}

// Tests for failure.

// TestRunIntentMonitorForFailure tests for failure.
func TestRunIntentMonitorForFailure(t *testing.T) {
	f := newIntentFixture(t)
	stopChannel := make(chan struct{})
	defer close(stopChannel)

	intent := newIntent("bar", 100)

	// syncHandler bails out.
	mon, faker := f.newMonitor(stopChannel)
	mon.syncHandler = func(key string) error {
		return errors.New("oh")
	}

	// TODO: assert that intent got added back to queue.
	go mon.Run(1, stopChannel)
	faker.Add(intent)
}

// TestProcessIntenForFailure tests for failure.
func TestProcessIntenForFailure(t *testing.T) {
	f := newIntentFixture(t)

	// non-existing object - no updates send.
	f.testSyncHandler("default/my-intent")

	// invalid key - no updates send.
	f.testSyncHandler("default/foo/bar")
}

// Tests for sanity.

// TestRunIntentMonitorForSanity tests for sanity.
func TestRunIntentMonitorForSanity(t *testing.T) {
	f := newIntentFixture(t)

	stopChannel := make(chan struct{})
	defer close(stopChannel)

	mon, faker := f.newMonitor(stopChannel)

	// syncHandler "replaced" by sth simpler...
	mon.syncHandler = func(key string) error {
		return nil
	}

	intent := newIntent("my-intent", 100)
	go mon.Run(1, stopChannel)
	faker.Add(intent)
	faker.Modify(intent)
	updatedIntent := intent.DeepCopy()
	updatedIntent.ResourceVersion = "foobar"
	faker.Modify(updatedIntent)
	faker.Delete(updatedIntent)

	// assert we got an update call.
	time.Sleep(timeout * time.Millisecond)
	intentUpdateWaitGroup.Wait()
	if f.actualUpdates[0].Priority != -1 {
		t.Error(fmt.Errorf("expected send an update to delete this intent"))
	}
}

// TestProcessIntentForSanity tests for sanity.
func TestProcessIntentForSanity(t *testing.T) {
	f := newIntentFixture(t)
	intent := newIntent("bar", 100)
	f.objects = append(f.objects, intent)
	f.intentLister = append(f.intentLister, intent)

	// simple update.
	f.expectedUpdates = []common.Intent{{Key: "default/bar"}}
	f.testSyncHandler("default/bar")

	secondIntent := intent.DeepCopy()
	secondIntent.Name = "foo"
	secondIntent.Spec.Objectives = append(intent.Spec.Objectives, v1alpha1.TargetObjective{Name: "my-p99", Value: 10, MeasuredBy: "p99latency"})
	f.objects = append(f.objects, secondIntent)
	f.intentLister = append(f.intentLister, secondIntent)
	f.expectedUpdates = []common.Intent{{Key: "default/foo"}}
	f.testSyncHandler("default/foo")
}
