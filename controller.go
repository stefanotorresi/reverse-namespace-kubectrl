package main

import (
	"fmt"
	"strings"
	"time"

	coreApi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaApi "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/apimachinery/pkg/util/wait"
	coreInformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	coreTyped "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"reversed-namespaces-kubectrl/pkg/stringutils"
)

const controllerAgentName = "reversed-namespaces-controller"

const (
	EventNamespaceAdded   = "NamespaceAdded"
	EventNamespaceDeleted = "NamespaceDeleted"
	EventReverseCreated   = "ReverseCreated"
	EventReverseDeleted   = "ReverseDeleted"
	MessageReverseCreated = "Reverse namespace created successfully"
	MessageReverseDeleted = "Reverse namespace deleted successfully"
)

type Controller struct {
	informer      coreInformers.NamespaceInformer
	kube          kubernetes.Interface
	workQueue     workqueue.RateLimitingInterface
	eventRecorder record.EventRecorder
	synced        cache.InformerSynced
}

func NewController(kube kubernetes.Interface, informer coreInformers.NamespaceInformer) *Controller {
	workQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName)
	eventRecorder := createEventRecorder(kube)

	controller := &Controller{
		informer:      informer,
		kube:          kube,
		workQueue:     workQueue,
		eventRecorder: eventRecorder,
		synced:        informer.Informer().HasSynced,
	}

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(numWorkers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workQueue.ShutDown()

	klog.Infof("Starting %s", controllerAgentName)

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.synced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			namespace := obj.(*coreApi.Namespace)
			c.workQueue.Add(serializeCacheKey(EventNamespaceAdded, namespace.Name))
		},
		DeleteFunc: func(obj interface{}) {
			namespace := obj.(*coreApi.Namespace)
			c.workQueue.Add(serializeCacheKey(EventNamespaceDeleted, namespace.Name))
		},
	})

	klog.Info("Starting workers")

	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually consume items from the workqueue.
func (c *Controller) runWorker() {
	for {
		obj, shutdown := c.workQueue.Get()

		if shutdown {
			break
		}

		// We wrap this block in a func so we can defer c.workQueue.Done.
		err := func(obj interface{}) error {
			defer c.workQueue.Done(obj)
			workItem, ok := obj.(string)

			if !ok {
				// As the item in the workqueue is actually invalid, we call
				// Forget here else we'd go into a loop of attempting to
				// process a work item that is invalid.
				c.workQueue.Forget(workItem)
				utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", workItem))
				return nil
			}

			if err := c.processWorkItem(workItem); err != nil {
				// Put the item back on the workqueue to handle any transient errors.
				c.workQueue.AddRateLimited(workItem)
				return fmt.Errorf("error processing '%s': %s, requeuing", workItem, err.Error())
			}
			// Finally, if no error occurs we Forget this item so it does not
			// get queued again until another change happens.
			c.workQueue.Forget(workItem)
			klog.Infof("Successfully processed '%s'", workItem)
			return nil
		}(obj)

		if err != nil {
			utilruntime.HandleError(err)
		}
	}
}

// asynchronously process a single work item from the queue
func (c *Controller) processWorkItem(key string) error {
	klog.Infof("Processing '%s'", key)

	var err error

	event, namespace := unserializeCacheKey(key)

	switch event {
	case EventNamespaceAdded:
		err = c.addReverse(namespace)
		break
	case EventNamespaceDeleted:
		err = c.deleteReverse(namespace)
		break
	default:
		utilruntime.HandleError(fmt.Errorf("unsupported work item with event '%s'", event))
		return nil
	}

	return err
}

func (c *Controller) addReverse(namespace string) error {
	reversedName := stringutils.Reverse(namespace)
	reversedNamespace, err := c.informer.Lister().Get(reversedName)

	if err == nil {
		klog.Infof("Reverse namespace '%s' already exists, ignoring", reversedName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	reversedNamespace, err = c.kube.CoreV1().Namespaces().Create(&coreApi.Namespace{
		ObjectMeta: metaApi.ObjectMeta{
			Name: reversedName,
		},
	})

	if err != nil {
		return err
	}

	c.eventRecorder.Event(reversedNamespace, coreApi.EventTypeNormal, EventReverseCreated, MessageReverseCreated)

	return nil
}

func (c *Controller) deleteReverse(namespace string) error {
	reversedName := stringutils.Reverse(namespace)
	reversedNamespace, err := c.informer.Lister().Get(reversedName)

	if errors.IsNotFound(err) {
		klog.Infof("Reverse namespace '%s' no longer exists, ignoring", reversedName)
		return nil
	}

	if err != nil {
		return err
	}

	err = c.kube.CoreV1().Namespaces().Delete(reversedName, &metaApi.DeleteOptions{})

	if err != nil {
		return err
	}

	c.eventRecorder.Event(reversedNamespace, coreApi.EventTypeNormal, EventReverseDeleted, MessageReverseDeleted)

	return nil
}

func serializeCacheKey(action string, namespace string) string {
	return fmt.Sprintf("%s %s", action, namespace)
}

func unserializeCacheKey(key string) (action string, namespace string) {
	parts := strings.SplitN(key, " ", 2)
	return parts[0], parts[1]
}

func createEventRecorder(kube kubernetes.Interface) record.EventRecorder {
	klog.Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&coreTyped.EventSinkImpl{Interface: kube.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, coreApi.EventSource{Component: controllerAgentName})
	return eventRecorder
}
