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
	"k8s.io/client-go/informers"
	coreInformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	coreTyped "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"reverse-namespace-kubectrl/pkg/stringutils"
)

const controllerAgentName = "reverse-namespace-controller"

const (
	EventNamespaceAdded   = "NamespaceAdded"
	EventNamespaceDeleted = "NamespaceDeleted"
	EventReverseCreated   = "ReverseCreated"
	EventReverseDeleted   = "ReverseDeleted"
	MessageReverseCreated = "Reverse namespace created successfully"
	MessageReverseDeleted = "Reverse namespace deleted successfully"
)

type Controller struct {
	informerFactory informers.SharedInformerFactory
	informer        coreInformers.NamespaceInformer
	kube            kubernetes.Interface
	workQueue       workqueue.RateLimitingInterface
	eventRecorder   record.EventRecorder
	synced          cache.InformerSynced
}

func NewController(kube kubernetes.Interface, informerFactory informers.SharedInformerFactory) *Controller {
	workQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName)
	eventRecorder := createEventRecorder(kube)
	informer := informerFactory.Core().V1().Namespaces()

	controller := &Controller{
		informer:        informer,
		informerFactory: informerFactory,
		kube:            kube,
		workQueue:       workQueue,
		eventRecorder:   eventRecorder,
		synced:          informer.Informer().HasSynced,
	}

	return controller
}

/*
Start the informer, set up the event handlers for types we are interested in,
as well as syncing informer caches and starting workers. It will block until stop
is closed, at which point it will shutdown the workqueue and wait for
workers to finish processing their current work items.
 */
func (c *Controller) Run(numWorkers int, stop <-chan struct{}) error {
	c.informerFactory.Start(stop)

	defer utilruntime.HandleCrash()
	defer c.workQueue.ShutDown()

	klog.Infof("Starting %s", controllerAgentName)

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stop, c.synced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			namespace := obj.(*coreApi.Namespace)
			c.workQueue.Add(serializeCacheKey(EventNamespaceAdded, namespace))
		},
		DeleteFunc: func(obj interface{}) {
			namespace := obj.(*coreApi.Namespace)
			c.workQueue.Add(serializeCacheKey(EventNamespaceDeleted, namespace))
		},
	})

	klog.Info("Starting workers")

	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stop)
	}

	klog.Info("Started workers")
	<-stop
	klog.Info("Shutting down workers")

	return nil
}

/*
A long-running function that will continually consume items from the workqueue.
It is invoked asynchronously by Run
*/
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

// process a single work item from the queue
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
	reverseName := stringutils.Reverse(namespace)
	reverseNamespace, err := c.informer.Lister().Get(reverseName)

	if err == nil {
		klog.Infof("Reverse namespace '%s' already exists, ignoring", reverseName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	reverseNamespace, err = c.kube.CoreV1().Namespaces().Create(&coreApi.Namespace{
		ObjectMeta: metaApi.ObjectMeta{
			Name: reverseName,
		},
	})

	if err != nil {
		return err
	}

	c.eventRecorder.Event(reverseNamespace, coreApi.EventTypeNormal, EventReverseCreated, MessageReverseCreated)

	return nil
}

func (c *Controller) deleteReverse(namespace string) error {
	reverseName := stringutils.Reverse(namespace)
	reverseNamespace, err := c.informer.Lister().Get(reverseName)

	if errors.IsNotFound(err) {
		klog.Infof("Reverse namespace '%s' no longer exists, ignoring", reverseName)
		return nil
	}

	if err != nil {
		return err
	}

	err = c.kube.CoreV1().Namespaces().Delete(reverseName, &metaApi.DeleteOptions{})

	if err != nil {
		return err
	}

	c.eventRecorder.Event(reverseNamespace, coreApi.EventTypeNormal, EventReverseDeleted, MessageReverseDeleted)

	return nil
}

func serializeCacheKey(action string, namespace *coreApi.Namespace) string {
	return fmt.Sprintf("%s %s", action, namespace.Name)
}

func unserializeCacheKey(key string) (action string, namespaceName string) {
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
