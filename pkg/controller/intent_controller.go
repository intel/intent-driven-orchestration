package controller

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/klog/v2"
)

// lockFile defines a path to the lock file.
const lockFile = "/tmp/trainings_lock"

// warmupDone will be set to true once the first tick is triggered.
var warmupDone = false

// warmupLock is a RW mutex for the warmupDone flag.
var warmupLock = sync.RWMutex{}

// IntentController defines the overall intent controller.
type IntentController struct {
	cfg          common.Config
	clientSet    kubernetes.Interface
	podInformer  v1.PodInformer
	tasks        chan string
	intents      map[string]common.Intent
	intentsLock  sync.RWMutex
	profiles     map[string]common.Profile
	profilesLock sync.Mutex
	podErrors    map[string][]common.PodError
	podErrorLock sync.Mutex
	planner      planner.Planner
	tracer       Tracer
	planCache    *common.TTLCache
	plannerMutex sync.RWMutex
}

// NewController initializes a new IntentController.
func NewController(cfg common.Config, tracer Tracer, clientSet kubernetes.Interface, informer v1.PodInformer) *IntentController {
	if cfg.Controller.TaskChannelLength <= 0 ||
		cfg.Controller.TaskChannelLength > common.MaxTaskChannelLen {
		klog.Error("invalid input value. Check documentation for the allowed limit")
		return nil
	}
	taskChannel := make(chan string, cfg.Controller.TaskChannelLength)
	c := &IntentController{
		cfg:         cfg,
		clientSet:   clientSet,
		podInformer: informer,
		tasks:       taskChannel,
		intents:     make(map[string]common.Intent),
		profiles:    make(map[string]common.Profile),
		podErrors:   make(map[string][]common.PodError),
		tracer:      tracer,
	}
	c.planCache, _ = common.NewCache(cfg.Controller.PlanCacheTTL, time.Duration(cfg.Controller.PlanCacheTimeout))
	return c
}

// SetPlanner sets planner used by all workers. This function is thread-safe, it blocks until the planner
// is set. All jobs run before SetPlanner call will finish their computations using previous planner.
func (c *IntentController) SetPlanner(planner planner.Planner) {
	c.plannerMutex.Lock()
	defer c.plannerMutex.Unlock()
	c.planner = planner
}

func (c *IntentController) getPlanner() planner.Planner {
	c.plannerMutex.RLock()
	defer c.plannerMutex.RUnlock()
	planner := c.planner
	return planner
}

// UpdateIntent channel function used by the intent monitor so send updates.
func (c *IntentController) UpdateIntent() chan<- common.Intent {
	events := make(chan common.Intent)
	go func() {
		for e := range events {
			c.intentsLock.Lock()
			if e.Priority >= 0 {
				c.intents[e.Key] = e
			} else {
				delete(c.intents, e.Key)
			}
			c.intentsLock.Unlock()
			c.processIntents()
		}
	}()
	return events
}

// UpdateProfile channel function used by the profile monitor to send updates.
func (c *IntentController) UpdateProfile() chan<- common.Profile {
	events := make(chan common.Profile)
	go func() {
		for e := range events {
			c.profilesLock.Lock()
			if e.ProfileType > common.Obsolete {
				c.profiles[e.Key] = e
			} else {
				delete(c.profiles, e.Key)
			}
			c.profilesLock.Unlock()
			c.processIntents()
		}
	}()
	return events
}

// UpdatePodError channel function used by the pod monitor to send updates.
func (c *IntentController) UpdatePodError() chan<- common.PodError {
	events := make(chan common.PodError)
	go func() {
		for e := range events {
			c.podErrorLock.Lock()
			if e.Start.IsZero() {
				delete(c.podErrors, e.Key)
			} else {
				c.podErrors[e.Key] = append(c.podErrors[e.Key], e)
			}
			c.podErrorLock.Unlock()
			c.processIntents()
		}
	}()
	return events
}

// processIntents triggers processing of all intents currently known.
func (c *IntentController) processIntents() {
	warmupLock.Lock()
	tmp := warmupDone
	warmupLock.Unlock()
	if tmp {
		c.intentsLock.Lock()
		for key := range c.intents {
			if !c.planCache.IsIn(key) {
				// only need to trigger planner when we've not recently triggered a plan execution.
				c.tasks <- key
			}
		}
		c.intentsLock.Unlock()
	}
}

// worker will trigger the planner to look into an intent.
func (c *IntentController) worker(id int, tasks <-chan string) {
	for key := range tasks {
		klog.V(2).Infof("Worker %d looking at: %s.", id, key)
		planner := c.getPlanner()
		if planner == nil {
			klog.Info("no planner configured")
			continue
		}
		c.intentsLock.Lock()
		current := getCurrentState(c.cfg.Controller, c.clientSet, c.podInformer, c.intents[key], c.podErrors, c.profiles)
		desired := getDesiredState(c.intents[key])
		c.intentsLock.Unlock()
		plan := planner.CreatePlan(current, desired, c.profiles)
		klog.Infof("Planner output for %s was: %v", key, plan)
		_, err := os.Stat(lockFile)
		if err != nil && len(plan) > 0 {
			klog.V(2).Infof("Triggering execution of plan for: %s.", key)
			go planner.ExecutePlan(current, plan)
			c.planCache.Put(key)
		}
		klog.V(2).Infof("Triggering effect calculation for: %s.", key)
		go planner.TriggerEffect(current, c.profiles)
		klog.V(2).Infof("Tracing event for: %s.", key)
		c.tracer.TraceEvent(current, desired, plan)
	}
}

// Run the overall IntentController logic.
func (c *IntentController) Run(nWorkers int, stopper <-chan struct{}) {
	for i := 0; i < nWorkers; i++ {
		go c.worker(i, c.tasks)
	}
	klog.V(1).Infof("Started %d worker(s).", nWorkers)

	ticker := time.NewTicker(time.Duration(c.cfg.Controller.ControllerTimeout) * time.Second)
	go func() {
		for {
			select {
			case <-stopper:
				return
			case t := <-ticker.C:
				klog.V(2).Infof("Tick at: %s", t)
				warmupLock.Lock()
				if !warmupDone {
					// This is stupid - to many ifs; but works for now.
					warmupDone = true
				}
				warmupLock.Unlock()
				c.processIntents()
			}
			runtime.Gosched()
		}
	}()
}
