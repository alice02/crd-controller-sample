package main

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	sampleresourcev1 "github.com/alice02/crd-controller-sample/pkg/apis/mysamplecontroller/v1"
	clientset "github.com/alice02/crd-controller-sample/pkg/client/clientset/versioned"
	samplescheme "github.com/alice02/crd-controller-sample/pkg/client/clientset/versioned/scheme"
	informers "github.com/alice02/crd-controller-sample/pkg/client/informers/externalversions/mysamplecontroller/v1"
	listers "github.com/alice02/crd-controller-sample/pkg/client/listers/mysamplecontroller/v1"
)

const controllerAgentName = "alice02-sample-controller"

const (
	SuccessSynced     = "Synced"
	ErrResourceExists = "ErrResourceExists"

	MessageResourceExists = "Resource %q alreadt exists and is not managed by SampleResource"
	MessageResourceSynced = "Sample synced successfully"
)

type Controller struct {
	kubeclientset   kubernetes.Interface
	sampleclientset clientset.Interface

	// deploymentsLister    appslisters.DeplomentLister
	// deploymentsSynced    cache.InformerSynced
	sampleResourceLister listers.SampleResourceLister
	sampleResourceSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
	recorder  record.EventRecorder
}

func NewController(
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,
	//	deploymentInformer appsinformers.DeploymentInformer,
	sampleInformer informers.SampleResourceInformer,
) *Controller {
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:   kubeclientset,
		sampleclientset: sampleclientset,
		// deploymentsLister:    deploymentInformer.Lister(),
		// deploymentsSynced:    deploymentInformer.Informer().HasSynced,
		sampleResourceLister: sampleInformer.Lister(),
		sampleResourceSynced: sampleInformer.Informer().HasSynced,
		workqueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "SampleResource"),
		recorder:             recorder,
	}

	klog.Info("Setting up event handlers")

	sampleInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: controller.enqueueSampleResource,
			UpdateFunc: func(old, new interface{}) {
				controller.enqueueSampleResource(new)
			},
		},
	)

	// deploymentInformer.Informer().AddEventHandler(
	// 	cache.ResourceEventHanderFuncs{
	// 		AddFunc: controller.handleObject,
	// 		UpdateFunc: func(old, new interface{}) {
	// 			newDepl := new.(*appsv1.Deployment)
	// 			oldDepl := old.(*appsv1.Deployment)
	// 			if newDepl.ReourceVersion == oldDepl.ResourceVersion {
	// 				return
	// 			}
	// 			controller.handleObject(new)
	// 		},
	// 		DeleteFunc: controller.handleObject,
	// 	},
	// )

	return controller
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting SampleResource Controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.sampleResourceSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := c.syncHandler(key); err != nil {
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.workqueue.Forget(obj)
		klog.Info("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) enqueueSampleResource(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}

	c.workqueue.AddRateLimited(key)
}

func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	sr, err := c.sampleResourceLister.SampleResources(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("sample resource '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	srName := sr.Spec.Name
	if srName == "" {
		runtime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	err = c.updateSampleResourceStatus(sr)
	if err != nil {
		return err
	}

	c.recorder.Event(sr, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateSampleResourceStatus(sr *sampleresourcev1.SampleResource) error {
	srCopy := sr.DeepCopy()
	srCopy.Status.Name = sr.Spec.Name
	_, err := c.sampleclientset.ExampleV1().SampleResources(sr.Namespace).Update(srCopy)
	return err
}
