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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
)

// profileUpdateWaitGroup is a silly way to make sure we don't have race conditions in our test code.
var profileUpdateWaitGroup sync.WaitGroup

// timeout for testing.
const timeout = 10

// profileFixture basic test struct.
type profileFixture struct {
	test                 *testing.T
	client               *fake.Clientset
	profileLister        []*v1alpha1.KPIProfile
	objects              []runtime.Object
	expectedActions      []core.Action
	actualUpdates        []common.Profile
	expectedUpdatesTypes []common.ProfileType
}

// newProfileFixture initializes the test struct.
func newProfileFixture(test *testing.T) *profileFixture {
	f := &profileFixture{}
	f.test = test
	return f
}

// acceptUpdates channel function to accept profile actualUpdates.
func (f *profileFixture) acceptUpdates() chan<- common.Profile {
	events := make(chan common.Profile)
	go func() {
		for {
			e := <-events
			profileUpdateWaitGroup.Add(1)
			f.actualUpdates = append(f.actualUpdates, e)
			profileUpdateWaitGroup.Done()
		}
	}()
	return events
}

// newMonitor creates a new monitor tailored for testing.
func (f *profileFixture) newMonitor(stopper chan struct{}) (*KPIProfileMonitor, *watch.FakeWatcher) {
	// new client...
	f.client = fake.NewSimpleClientset(f.objects...)
	fakeWatch := watch.NewFake()
	f.client.PrependWatchReactor("kpiprofiles", core.DefaultWatchReactor(fakeWatch, nil))

	// new monitor
	informer := informers.NewSharedInformerFactory(f.client, func() time.Duration { return 0 }())
	mon := NewKPIProfileMonitor(
		common.MonitorConfig{Profile: struct {
			Workers int    `json:"workers"`
			Queries string `json:"queries"`
		}(struct {
			Workers int
			Queries string
		}{Queries: "../../artefacts/examples/default_queries.json"})},
		f.client,
		informer.Ido().V1alpha1().KPIProfiles(),
		f.acceptUpdates())
	mon.profileSynced = func() bool { return true }

	for _, f := range f.profileLister {
		err := informer.Ido().V1alpha1().KPIProfiles().Informer().GetIndexer().Add(f)
		if err != nil {
			klog.Fatal(err)
		}
	}

	informer.Start(stopper)
	return mon, fakeWatch
}

// getActualActions returns the actual actions that happened - except the watch and list ones.
func (f *profileFixture) getActualActions() []core.Action {
	var res []core.Action
	for _, action := range f.client.Actions() {
		if action.GetVerb() == "list" || action.GetVerb() == "watch" {
			// no need to check for these.
			continue
		}
		res = append(res, action)
	}
	return res
}

// testSyncHandler triggers the objectiveController and checks for correct behaviour.
func (f *profileFixture) testSyncHandler(key string) {
	stopCh := make(chan struct{})
	defer close(stopCh)
	mon, _ := f.newMonitor(stopCh)

	// check if it threw an error around.
	err := mon.syncHandler(key)
	if err != nil {
		f.test.Error(fmt.Errorf("error running test: %s", err))
	}

	// make sure the updates are always captured.
	time.Sleep(timeout * time.Millisecond)
	profileUpdateWaitGroup.Wait()

	// check if actualActions happened.
	actualActions := f.getActualActions()
	j := 0
	for i, actualAction := range actualActions {
		expectedAction := f.expectedActions[i]
		if !(expectedAction.Matches(actualAction.GetVerb(), actualAction.GetResource().Resource)) {
			f.test.Errorf("Action mismach: %s - %s.", actualAction, expectedAction)
			break
		}
		j = 1
	}
	if j != len(f.expectedActions) {
		f.test.Errorf("Number of actual and expected action(s) mismatched: %d - %d.", len(actualActions), len(f.expectedActions))
	}

	// check if updates came through.
	for i, actualUpdate := range f.actualUpdates {
		if actualUpdate.ProfileType != f.expectedUpdatesTypes[i] {
			f.test.Errorf("Update mismatch: %d - %d.", actualUpdate.ProfileType, f.expectedUpdatesTypes[i])
			break
		}
	}
	if len(f.actualUpdates) != len(f.expectedUpdatesTypes) {
		f.test.Errorf("Number of actual and expected update(s) mismatched: %d - %d.", len(f.actualUpdates), len(f.expectedUpdatesTypes))
	}

	// reset!
	f.actualUpdates = make([]common.Profile, 0)
	f.expectedActions = make([]core.Action, 0)
	f.expectedUpdatesTypes = make([]common.ProfileType, 0)
	f.client.ClearActions()
}

// newKPIProfile creates a dummy KPIProfile.
func newKPIProfile(name string, kind string, query string, smaller bool, endpoint string) *v1alpha1.KPIProfile {
	return &v1alpha1.KPIProfile{
		TypeMeta: metaV1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: metaV1.NamespaceDefault,
		},
		Spec: v1alpha1.KPIProfileSpec{
			Query:    query,
			KPIType:  kind,
			Minimize: smaller,
			Props:    map[string]string{"endpoint": endpoint},
		},
	}
}

// Tests for success.

// TestRunForSuccess tests for success.
func TestRunKPIProfileMonitorForSuccess(t *testing.T) {
	f := newProfileFixture(t)

	foo := newKPIProfile("foo", "", "", true, "")
	f.profileLister = append(f.profileLister, foo)
	f.objects = append(f.objects, foo)

	stopChannel := make(chan struct{})
	defer close(stopChannel)
	mon, faker := f.newMonitor(stopChannel)

	// syncHandler "replaced" by sth simpler...
	mon.syncHandler = func(_ string) error {
		return nil
	}

	go mon.Run(1, stopChannel)
	faker.Add(foo)
}

// TestProcessProfileForSuccess tests for success.
func TestProcessProfileForSuccess(t *testing.T) {
	f := newProfileFixture(t)

	// new objective profile.
	newProfile := newKPIProfile("availability", "availability", "", true, "")
	f.profileLister = append(f.profileLister, newProfile)
	f.objects = append(f.objects, newProfile)

	// add expected action(s).
	action := core.NewUpdateSubresourceAction(
		schema.GroupVersionResource{Resource: "kpiprofiles"},
		"status",
		newProfile.Namespace,
		newProfile)
	f.expectedActions = append(f.expectedActions, action)

	// add expected result.
	f.expectedUpdatesTypes = append(f.expectedUpdatesTypes, common.ProfileTypeFromText("availability"))

	// run this test.
	key := "default/availability"
	f.testSyncHandler(key)
}

// Tests for failure.

// TestRunKPIProfileMonitorForFailure tests for failure.
func TestRunKPIProfileMonitorForFailure(t *testing.T) {
	f := newProfileFixture(t)
	stopChannel := make(chan struct{})
	defer close(stopChannel)

	newProfile := newKPIProfile("availability", "availability", "", true, "")

	// syncHandler bails out.
	mon, faker := f.newMonitor(stopChannel)
	mon.syncHandler = func(_ string) error {
		return errors.New("whoops")
	}

	// TODO: assert that profile got added back to queue.
	go mon.Run(1, stopChannel)
	faker.Add(newProfile)
}

// TestProcessProfileForFailure tests for failure.
func TestProcessProfileForFailure(t *testing.T) {
	f := newProfileFixture(t)

	// non-existing object - no updates send.
	f.testSyncHandler("default/availability")

	// invalid key - no updates send.
	f.testSyncHandler("availability/foo/bar")
}

// Tests for sanity.

// TestRunKPIProfileMonitorForSanity tests for sanity.
func TestRunKPIProfileMonitorForSanity(t *testing.T) {
	f := newProfileFixture(t)

	stopChannel := make(chan struct{})
	defer close(stopChannel)

	mon, faker := f.newMonitor(stopChannel)

	// syncHandler "replaced" by sth simpler...
	mon.syncHandler = func(_ string) error {
		return nil
	}

	profile := newKPIProfile("p50latency", "latency", "", true, "")
	go mon.Run(1, stopChannel)
	faker.Add(profile)
	updateProfile := profile.DeepCopy()
	updateProfile.ResourceVersion = "foobar"
	faker.Modify(updateProfile)
	faker.Delete(updateProfile)

	// assert we got an update call.
	time.Sleep(timeout * time.Millisecond)
	profileUpdateWaitGroup.Wait()
	if f.actualUpdates[0].ProfileType != common.Obsolete {
		t.Error(fmt.Errorf("expected update action to be called at the end"))
	}
}

// TestProcessProfileForSanity tests for sanity.
func TestProcessProfileForSanity(t *testing.T) {
	f := newProfileFixture(t)

	// new objective profile.
	newProfile := newKPIProfile("my-fps", "throughput", "abc", true, "https://foo:8080")
	f.profileLister = append(f.profileLister, newProfile)
	f.objects = append(f.objects, newProfile)

	// add expected action(s).
	action := core.NewUpdateSubresourceAction(
		schema.GroupVersionResource{Resource: "kpiprofiles"},
		"status",
		newProfile.Namespace,
		newProfile)
	f.expectedActions = append(f.expectedActions, action)

	// add expected update to be sent to channel.
	f.expectedUpdatesTypes = append(f.expectedUpdatesTypes, common.ProfileTypeFromText("throughput"))

	// run
	f.testSyncHandler("default/my-fps")

	// now let's add an invalid objective profile...
	f = newProfileFixture(t)
	secondProfile := newKPIProfile("my-lat", "latency", "", true, "")
	f.profileLister = append(f.profileLister, secondProfile)
	f.objects = append(f.objects, secondProfile)
	action = core.NewUpdateSubresourceAction(
		schema.GroupVersionResource{Resource: "kpiprofiles"},
		"status",
		newProfile.Namespace,
		newProfile)
	f.expectedActions = append(f.expectedActions, action)

	// expecting an obsolete being pushed down.
	f.expectedUpdatesTypes = append(f.expectedUpdatesTypes, common.ProfileTypeFromText("obsolete"))

	// run
	f.testSyncHandler("default/my-lat")
}
