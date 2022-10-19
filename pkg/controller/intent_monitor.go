package controller

import (
	"fmt"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/api/intents/v1alpha1"
	"github.com/intel/intent-driven-orchestration/pkg/common"
	clientSet "github.com/intel/intent-driven-orchestration/pkg/generated/clientset/versioned"
	informers "github.com/intel/intent-driven-orchestration/pkg/generated/informers/externalversions/intents/v1alpha1"
	lister "github.com/intel/intent-driven-orchestration/pkg/generated/listers/intents/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// IntentMonitor is the part implementing the monitoring of Intents.
type IntentMonitor struct {
	intentClient clientSet.Interface
	intentLister lister.IntentLister
	intentSynced cache.InformerSynced
	queue        workqueue.RateLimitingInterface
	update       chan<- common.Intent
	syncHandler  func(key string) error // For testing purposes.
}

// NewIntentMonitor returns a new monitor instance.
func NewIntentMonitor(intentClient clientSet.Interface, intentInformer informers.IntentInformer, ch chan<- common.Intent) *IntentMonitor {
	mon := &IntentMonitor{
		intentClient: intentClient,
		intentLister: intentInformer.Lister(),
		intentSynced: intentInformer.Informer().HasSynced,
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Intents"),
		update:       ch,
	}
	mon.syncHandler = mon.processIntent

	// functions handler.
	intentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: mon.enqueueItem,
		UpdateFunc: func(old, new interface{}) {
			if old.(*v1alpha1.Intent).ResourceVersion == new.(*v1alpha1.Intent).ResourceVersion {
				return
			}
			mon.enqueueItem(new)
		},
		DeleteFunc: func(obj interface{}) {
			var key string
			var err error
			key, err = cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				runtime.HandleError(err)
				return
			}
			klog.Infof("Will remove intent: '%s'.", key)
			mon.update <- common.Intent{Key: key, Priority: -1.0}
		},
	})

	return mon
}

// enqueueItem adds items to the work queue.
func (mon *IntentMonitor) enqueueItem(obj interface{}) {
	var key string
	var err error
	key, err = cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	mon.queue.Add(key)
}

// Run the basic monitor.
func (mon *IntentMonitor) Run(nWorkers int, stopper <-chan struct{}) {
	defer runtime.HandleCrash()
	defer mon.queue.ShutDown()

	if ok := cache.WaitForCacheSync(stopper, mon.intentSynced); !ok {
		return
	}

	for i := 0; i < nWorkers; i++ {
		go wait.Until(mon.runWorker, time.Second, stopper)
	}
	klog.V(1).Infof("Started %d worker(s).", nWorkers)
	<-stopper
}

// runWorker will run forever and process items of a queue.
func (mon *IntentMonitor) runWorker() {
	for mon.processNextWorkItem() {
	}
}

// processNextWorkItem will handle item in the queue.
func (mon *IntentMonitor) processNextWorkItem() bool {
	obj, done := mon.queue.Get()
	if done {
		return false
	}
	defer mon.queue.Done(obj)

	// process obj.
	err := mon.syncHandler(obj.(string))
	if err == nil {
		mon.queue.Forget(obj)
		return true
	}

	// Failed --> add back to queue, but rate limited!
	runtime.HandleError(fmt.Errorf("processing of %v failed with: %v", obj, err))
	mon.queue.AddRateLimited(obj)

	return true
}

// processIntent processes an actual action performed on an intent.
func (mon *IntentMonitor) processIntent(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: '%s'", key))
		//lint:ignore nilerr n.a.
		return nil // ignore
	}

	intent, err := mon.intentLister.Intents(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("intent '%s' does not longer exists", key))
			return nil
		}
		return err
	}

	// easier to work with a map in the planner later on.
	objectivesMap := make(map[string]float64)
	for _, target := range intent.Spec.Objectives {
		if _, ok := objectivesMap[target.MeasuredBy]; ok {
			// TODO: set status to faulty.
			klog.Warningf("There are multiple targets with the same profile, that should not happen: %s.", target.Name)
		}
		objectivesMap[target.MeasuredBy] = target.Value
	}

	updateObject := common.Intent{
		Key:        key,
		Priority:   intent.Spec.Priority,
		TargetKey:  intent.Spec.TargetRef.Name,
		TargetKind: intent.Spec.TargetRef.Kind,
		Objectives: objectivesMap,
	}
	mon.update <- updateObject

	return nil
}
