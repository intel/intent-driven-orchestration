package controller

import (
	"fmt"
	"sync"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreInformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	coreLister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// PodMonitor struct.
type PodMonitor struct {
	podClient       kubernetes.Interface
	podLister       coreLister.PodLister
	podSynced       cache.InformerSynced
	queue           workqueue.RateLimitingInterface
	update          chan<- common.PodError
	podsWithError   map[string]bool
	syncHandler     func(key string) error // Enables us to test this easily.
	podCacheChannel chan<- podIsInError
	cacheLock       sync.Mutex
}

// podIsInError is a little helper struct to handle the cache.
type podIsInError struct {
	key   string
	state bool
}

// updatePodCache enables the worked to update the pod cache.
func (mon *PodMonitor) updatePodCache() chan<- podIsInError {
	cacheChannel := make(chan podIsInError)
	go func() {
		for e := range cacheChannel {
			mon.cacheLock.Lock()
			if !e.state {
				delete(mon.podsWithError, e.key)
			} else {
				mon.podsWithError[e.key] = true
			}
			mon.cacheLock.Unlock()
		}
	}()
	return cacheChannel
}

// NewPodMonitor initialize a new monitor for PODs - the foundation of a lot of what is going on in K8s/K3s.
func NewPodMonitor(podClient kubernetes.Interface, informer coreInformer.PodInformer, ch chan<- common.PodError) *PodMonitor {
	mon := &PodMonitor{
		podClient:     podClient,
		podLister:     informer.Lister(),
		podSynced:     informer.Informer().HasSynced,
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Pods"),
		update:        ch,
		podsWithError: make(map[string]bool),
		cacheLock:     sync.Mutex{},
	}
	mon.syncHandler = mon.processPod
	mon.podCacheChannel = mon.updatePodCache()

	// event handlers.
	_, _ = informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// No need to AddFunc, as POD is first pending...through an update is initially gets ready.
		UpdateFunc: func(old, new interface{}) {
			if old.(*coreV1.Pod).ResourceVersion == new.(*coreV1.Pod).ResourceVersion {
				// no change --> nothing to do.
				return
			}
			mon.enqueuePod(new)
		},
		DeleteFunc: func(obj interface{}) {
			var key string
			var err error
			key, err = cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				runtime.HandleError(err)
				return
			}
			klog.Infof("Will dump data on POD: '%s'.", key)
			mon.update <- common.PodError{Key: key}
		},
	})

	return mon
}

// enqueuePod adds an entry which is to be process to the queue.
func (mon *PodMonitor) enqueuePod(obj interface{}) {
	var key string
	var err error
	key, err = cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	mon.queue.Add(key)
}

// Run the basic monitors. Note that it is crucial to have enough workers, so you do not miss any errors as they are stuck in the queue.
func (mon *PodMonitor) Run(nWorkers int, stopper <-chan struct{}) {
	defer runtime.HandleCrash()
	defer mon.queue.ShutDown()

	if ok := cache.WaitForCacheSync(stopper, mon.podSynced); !ok {
		return
	}

	for i := 0; i < nWorkers; i++ {
		go wait.Until(mon.runWorker, time.Second, stopper)
	}
	klog.V(1).Infof("Started %d worker(s).", nWorkers)
	<-stopper
}

// runWorker will run forever and process items of a queue.
func (mon *PodMonitor) runWorker() {
	for mon.processNextWorkItem() {
	}
}

// processNextWorkItem will handle item in the queue.
func (mon *PodMonitor) processNextWorkItem() bool {
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
	runtime.HandleError(fmt.Errorf("%v failed with: %v", obj, err))
	mon.queue.AddRateLimited(obj)

	return true
}

// processPod actually looks at the POD and tries to determine errors that happened.
func (mon *PodMonitor) processPod(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: '%s'", key))
		//lint:ignore nilerr n.a.
		return nil // ignore
	}
	pod, err := mon.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("POD '%s' does not longer exists", key))
			return nil
		}
		return err
	}

	// FIXME: check handling of multiple containers - currently only looking at the first one.
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil
	}
	for _, state := range pod.Status.ContainerStatuses[:1] {
		// TODO: also include information from readiness probes.
		if _, ok := mon.podsWithError[key]; ok {
			// check if error is over.
			if state.Ready && state.LastTerminationState.Terminated != nil {
				start := state.LastTerminationState.Terminated.FinishedAt.Time // when the last container instance failed.
				end := state.State.Running.StartedAt.Time                      // when the new container was started.
				mon.update <- common.PodError{Key: key, Start: start, End: end, Created: pod.CreationTimestamp.Time}
				mon.podCacheChannel <- podIsInError{key, false}
				klog.Infof("POD '%s' was in error state from '%s' to '%s'.", key, start, end)
			}
		} else {
			// check if pod is in error state.
			if !state.Ready && state.LastTerminationState.Terminated != nil {
				mon.podCacheChannel <- podIsInError{key, true}
			}
		}
	}
	return nil
}
