package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/api/intents/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	clientSet "github.com/intel/intent-driven-orchestration/pkg/generated/clientset/versioned"
	informers "github.com/intel/intent-driven-orchestration/pkg/generated/informers/externalversions/intents/v1alpha1"
	lister "github.com/intel/intent-driven-orchestration/pkg/generated/listers/intents/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// KPIProfileMonitor is the part implementing the monitoring the KPIProfiles.
type KPIProfileMonitor struct {
	profileClient   clientSet.Interface
	profileLister   lister.KPIProfileLister
	profileSynced   cache.InformerSynced
	queue           workqueue.TypedRateLimitingInterface[string]
	update          chan<- common.Profile
	defaultProfiles map[string]map[string]string
	syncHandler     func(key string) error // Enables us to test this easily.
}

// NewKPIProfileMonitor returns a new monitor instance.
func NewKPIProfileMonitor(cfg common.MonitorConfig, profileClient clientSet.Interface, profileInformer informers.KPIProfileInformer, ch chan<- common.Profile) *KPIProfileMonitor {
	// parse default configs.
	tmp, err := os.ReadFile(cfg.Profile.Queries)
	if err != nil {
		klog.Fatal("Unable to read config file with the default profile definitions: ", err)
	}
	var result map[string]map[string]string
	err = json.Unmarshal(tmp, &result)
	if err != nil {
		klog.Fatal("Unable to parse default profile definitions: ", err)
	}

	// the actual monitor.
	mon := &KPIProfileMonitor{
		profileClient:   profileClient,
		profileLister:   profileInformer.Lister(),
		profileSynced:   profileInformer.Informer().HasSynced,
		queue:           workqueue.NewTypedRateLimitingQueueWithConfig[string](workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "KPIProfiles"}),
		update:          ch,
		defaultProfiles: result,
	}
	mon.syncHandler = mon.processProfile

	// TODO: check for event broadcasting.

	// handle add, update & delete.
	_, _ = profileInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: mon.enqueueItem,
		UpdateFunc: func(oldVersion, newVersion interface{}) {
			if oldVersion.(*v1alpha1.KPIProfile).ResourceVersion == newVersion.(*v1alpha1.KPIProfile).ResourceVersion {
				// no change --> nothing to do.
				return
			}
			mon.enqueueItem(newVersion)
		},
		DeleteFunc: func(obj interface{}) {
			var key string
			var err error
			key, err = cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				runtime.HandleError(err)
				return
			}
			klog.Infof("Will remove profile '%s'.", key)
			mon.update <- common.Profile{Key: key, ProfileType: common.Obsolete, External: true}
		},
	})

	return mon
}

// enqueueItem adds items to the work queue.
func (mon *KPIProfileMonitor) enqueueItem(obj interface{}) {
	var key string
	var err error
	key, err = cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	mon.queue.Add(key)
}

// Run the basic monitors.
func (mon *KPIProfileMonitor) Run(nWorkers int, stopper <-chan struct{}) {
	defer runtime.HandleCrash()
	defer mon.queue.ShutDown()

	if ok := cache.WaitForCacheSync(stopper, mon.profileSynced); !ok {
		return
	}

	for i := 0; i < nWorkers; i++ {
		go wait.Until(mon.runWorker, time.Second, stopper)
	}
	klog.V(1).Infof("Started %d worker(s).", nWorkers)
	<-stopper
}

// runWorker will run forever and process items of a queue.
func (mon *KPIProfileMonitor) runWorker() {
	for mon.processNextWorkItem() {
	}
}

// processNextWorkItem will handle item in the queue.
func (mon *KPIProfileMonitor) processNextWorkItem() bool {
	obj, done := mon.queue.Get()
	if done {
		return false
	}
	defer mon.queue.Done(obj)

	// process obj.
	err := mon.syncHandler(obj)
	if err == nil {
		mon.queue.Forget(obj)
		return true
	}

	// Failed --> add back to queue, but rate limited!
	runtime.HandleError(fmt.Errorf("processing of %v failed with: %v", obj, err))
	mon.queue.AddRateLimited(obj)

	return true
}

// processProfile checks if it can resolve a profile, and if so marks it as resolved.
func (mon *KPIProfileMonitor) processProfile(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: '%s'", key))
		// nolint:nilerr // n.a.
		return nil // ignore
	}

	profile, err := mon.profileLister.KPIProfiles(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("KPI profile '%s' does not longer exists", key))
			return nil
		}
		return err
	}

	// look into the profile we got and if it is valid push it into the channel.
	var parsedProfile common.Profile
	if _, found := mon.defaultProfiles[key]; found {
		tmp := mon.defaultProfiles[key]
		parsedProfile = common.Profile{Key: key, ProfileType: common.ProfileTypeFromText(profile.Spec.KPIType), Query: tmp["query"], Minimize: profile.Spec.Minimize, Address: tmp["endpoint"]}
		mon.updateStatus(profile, true, "ok")
		mon.update <- parsedProfile
	} else {
		if _, found := profile.Spec.Props["endpoint"]; found && profile.Spec.Query != "" {
			// FIXME - make sure whatever is put in query is safe, secure & valid (regex maybe?)
			parsedProfile = common.Profile{Key: key, ProfileType: common.ProfileTypeFromText(profile.Spec.KPIType), Query: profile.Spec.Query, Minimize: profile.Spec.Minimize, External: true, Address: profile.Spec.Props["endpoint"]}
			mon.updateStatus(profile, true, "ok")
			mon.update <- parsedProfile
		} else {
			mon.updateStatus(profile, false, "Both a endpoint and a query need to be defined.")
			mon.update <- common.Profile{Key: key, ProfileType: common.Obsolete, External: true}
		}
	}

	return nil
}

// updateStatus actualUpdates the status of the CRD.
func (mon *KPIProfileMonitor) updateStatus(profile *v1alpha1.KPIProfile, resolved bool, reason string) {
	profileCopy := profile.DeepCopy()
	profileCopy.Status.Resolved = resolved
	profileCopy.Status.Reason = reason
	klog.Infof("Set status for profile '%s/%s' to '%t' - reason: '%s'.", profile.Namespace, profile.Name, resolved, reason)
	_, err := mon.profileClient.IdoV1alpha1().KPIProfiles(profile.Namespace).UpdateStatus(context.TODO(), profileCopy, metaV1.UpdateOptions{})
	if err != nil {
		runtime.HandleError(fmt.Errorf("unable to update status subresource: %s", err))
	}
}
