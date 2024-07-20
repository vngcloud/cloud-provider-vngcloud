package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	cuongpigerutils "github.com/cuongpiger/joat/utils"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/client"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/ingress/config"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
	vErrors "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/errors"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/metadata"
	vngcloudutil "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/vngcloud"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/version"
	vconSdkClient "github.com/vngcloud/vngcloud-go-sdk/client"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/policy"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/extensions/secgroup_rule"
	apiv1 "k8s.io/api/core/v1"
	nwv1 "k8s.io/api/networking/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	nwlisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	klog "k8s.io/klog/v2"
)

type Expander struct {
	serviceConf *IngressConfig
	*utils.IngressInspect
}

// EventType type of event associated with an informer
type EventType string

const (
	CreateEvent EventType = "CREATE"
	UpdateEvent EventType = "UPDATE"
	DeleteEvent EventType = "DELETE"
	SyncEvent   EventType = "SYNC"
)

// Event holds the context of an event
type Event struct {
	Type   EventType
	Obj    interface{}
	oldObj interface{}
}

// Controller ...
type Controller struct {
	config     *config.Config
	kubeClient kubernetes.Interface

	stopCh              chan struct{}
	knownNodes          []*apiv1.Node
	queue               workqueue.RateLimitingInterface
	informer            informers.SharedInformerFactory
	recorder            record.EventRecorder
	ingressLister       nwlisters.IngressLister
	ingressListerSynced cache.InformerSynced
	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced
	nodeLister          corelisters.NodeLister
	nodeListerSynced    cache.InformerSynced

	// vks
	provider  *vconSdkClient.ProviderClient
	vLBSC     *vconSdkClient.ServiceClient
	vServerSC *vconSdkClient.ServiceClient
	extraInfo *vngcloudutil.ExtraInfo

	isReApplyNextTime   bool // it have a bug when update default pool member, set this to reapply when update pool member
	trackLBUpdate       *utils.UpdateTracker
	mu                  sync.Mutex
	numOfUpdatingThread int

	mu2     sync.Mutex
	queues  map[string][]interface{}
	workers map[string]chan bool
}

// NewController creates a new VngCloud Ingress controller.
func NewController(conf config.Config) *Controller {
	// initialize k8s client
	kubeClient, err := utils.CreateApiserverClient(conf.Kubernetes.ApiserverHost, conf.Kubernetes.KubeConfig)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"api_server":  conf.Kubernetes.ApiserverHost,
			"kube_config": conf.Kubernetes.KubeConfig,
			"error":       err,
		}).Fatal("failed to initialize kubernetes client")
	}

	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	nodeInformer := kubeInformerFactory.Core().V1().Nodes()
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{Component: "vngcloud-ingress-controller"})

	controller := &Controller{
		config:     &conf,
		kubeClient: kubeClient,

		queue:               queue,
		stopCh:              make(chan struct{}),
		informer:            kubeInformerFactory,
		recorder:            recorder,
		serviceLister:       serviceInformer.Lister(),
		serviceListerSynced: serviceInformer.Informer().HasSynced,
		nodeLister:          nodeInformer.Lister(),
		nodeListerSynced:    nodeInformer.Informer().HasSynced,
		knownNodes:          []*apiv1.Node{},
		trackLBUpdate:       utils.NewUpdateTracker(),
		numOfUpdatingThread: 0,
		queues:              make(map[string][]interface{}),
		workers:             make(map[string]chan bool),
	}

	ingInformer := kubeInformerFactory.Networking().V1().Ingresses()
	_, err = ingInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			addIng := obj.(*nwv1.Ingress)
			key := fmt.Sprintf("%s/%s", addIng.Namespace, addIng.Name)

			if !IsValid(addIng) {
				logrus.Infof("ignore ingress %s", key)
				return
			}

			recorder.Event(addIng, apiv1.EventTypeNormal, "Creating", fmt.Sprintf("Ingress %s", key))
			controller.queue.AddRateLimited(Event{Obj: addIng, Type: CreateEvent, oldObj: nil})
		},
		UpdateFunc: func(old, new interface{}) {
			newIng := new.(*nwv1.Ingress)
			oldIng := old.(*nwv1.Ingress)
			if newIng.ResourceVersion == oldIng.ResourceVersion {
				// Periodic resync will send update events for all known Ingresses.
				// Two different versions of the same Ingress will always have different RVs.
				return
			}
			newAnnotations := newIng.ObjectMeta.Annotations
			oldAnnotations := oldIng.ObjectMeta.Annotations
			delete(newAnnotations, "kubectl.kubernetes.io/last-applied-configuration")
			delete(oldAnnotations, "kubectl.kubernetes.io/last-applied-configuration")

			key := fmt.Sprintf("%s/%s", newIng.Namespace, newIng.Name)
			validOld := IsValid(oldIng)
			validCur := IsValid(newIng)
			if !validOld && validCur {
				recorder.Event(newIng, apiv1.EventTypeNormal, "Creating", fmt.Sprintf("Ingress %s", key))
				controller.queue.AddRateLimited(Event{Obj: newIng, Type: CreateEvent, oldObj: nil})
			} else if validOld && !validCur {
				recorder.Event(newIng, apiv1.EventTypeNormal, "Deleting", fmt.Sprintf("Ingress %s", key))
				controller.queue.AddRateLimited(Event{Obj: newIng, Type: DeleteEvent, oldObj: nil})
			} else if validCur && (!reflect.DeepEqual(newIng.Spec, oldIng.Spec) || !reflect.DeepEqual(newAnnotations, oldAnnotations)) {
				recorder.Event(newIng, apiv1.EventTypeNormal, "Updating", fmt.Sprintf("Ingress %s", key))
				controller.queue.AddRateLimited(Event{Obj: newIng, Type: UpdateEvent, oldObj: oldIng})
			} else {
				return
			}
		},
		DeleteFunc: func(obj interface{}) {
			delIng, ok := obj.(*nwv1.Ingress)
			if !ok {
				// If we reached here it means the ingress was deleted but its final state is unrecorded.
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					logrus.Errorf("couldn't get object from tombstone %#v", obj)
					return
				}
				delIng, ok = tombstone.Obj.(*nwv1.Ingress)
				if !ok {
					logrus.Errorf("Tombstone contained object that is not an Ingress: %#v", obj)
					return
				}
			}

			key := fmt.Sprintf("%s/%s", delIng.Namespace, delIng.Name)
			if !IsValid(delIng) {
				logrus.Infof("ignore ingress %s", key)
				return
			}

			recorder.Event(delIng, apiv1.EventTypeNormal, "Deleting", fmt.Sprintf("Ingress %s", key))
			controller.queue.AddRateLimited(Event{Obj: delIng, Type: DeleteEvent, oldObj: nil})
		},
	})

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("failed to initialize ingress")
	}

	controller.ingressLister = ingInformer.Lister()
	controller.ingressListerSynced = ingInformer.Informer().HasSynced

	return controller
}

// Start starts the vngcloud ingress controller.
func (c *Controller) Start() {
	klog.Infoln("------------ Start() ------------")
	klog.Infoln("ClusterName: ", c.getClusterName())
	klog.Infoln("ClusterID: ", c.getClusterID())
	defer close(c.stopCh)
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	logrus.Debug("starting Ingress controller")
	err := c.Init()
	if err != nil {
		logrus.Fatal("failed to init controller: ", err)
	}

	go c.informer.Start(c.stopCh)

	// wait for the caches to synchronize before starting the worker
	if !cache.WaitForCacheSync(c.stopCh, c.ingressListerSynced, c.serviceListerSynced, c.nodeListerSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	logrus.Info("ingress controller synced and ready")

	readyWorkerNodes, err := utils.ListNodeWithPredicate(c.nodeLister, make(map[string]string, 0))
	if err != nil {
		logrus.Errorf("Failed to retrieve current set of nodes from node lister: %v", err)
		return
	}
	c.knownNodes = readyWorkerNodes

	go wait.Until(c.runWorker, time.Second, c.stopCh)
	go wait.Until(c.nodeSyncLoop, 60*time.Second, c.stopCh)

	<-c.stopCh
}

func (s *Controller) addUpdatingThread() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.numOfUpdatingThread++
}

func (s *Controller) removeUpdatingThread() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.numOfUpdatingThread--
}

func (s *Controller) addEvent(event Event) {
	s.mu2.Lock()
	defer s.mu2.Unlock()

	ing := event.Obj.(*nwv1.Ingress)
	key := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)
	logrus.Infoln("Add event to key:", key)

	q, ok := s.queues[key]
	if !ok {
		q = []interface{}{}
		s.queues[key] = q
	}
	s.queues[key] = append(q, event)

	// If no worker exists for this key, start a new worker
	if _, ok := s.workers[key]; !ok {
		s.workers[key] = make(chan bool)
		go s.startWorker(key)
	}

}

func (s *Controller) startWorker(key string) {
	logrus.Infof("Worker %s is running", key)
	for {
		select {
		case <-s.workers[key]:
			logrus.Infof("Worker %s is stop", key)
			delete(s.workers, key)
			return
		default:
			s.processNextItemInQueue(key)
		}
	}
}

func (s *Controller) processNextItemInQueue(key string) {
	q, ok := s.queues[key]
	if !ok || len(q) == 0 {
		// No items in the queue, stop the worker
		close(s.workers[key])
		return
	}

	obj := q[0]
	s.queues[key] = q[1:]

	s.processItem(obj.(Event))
}

// nodeSyncLoop handles updating the hosts pointed to by all load
// balancers whenever the set of nodes in the cluster changes.
func (c *Controller) nodeSyncLoop() {
	klog.Infoln("------------ nodeSyncLoop() ------------")
	if c.numOfUpdatingThread > 0 {
		klog.Infof("Skip nodeSyncLoop() because the controller is in the update mode.")
		return
	}
	readyWorkerNodes, err := utils.ListNodeWithPredicate(c.nodeLister, make(map[string]string, 0))
	if err != nil {
		logrus.Errorf("Failed to retrieve current set of nodes from node lister: %v", err)
		return
	}

	isReApply := false
	if c.isReApplyNextTime {
		c.isReApplyNextTime = false
		isReApply = true
	}

	if !isReApply && !utils.NodeSlicesEqual(readyWorkerNodes, c.knownNodes) {
		isReApply = true
		logrus.Infof("Detected change in list of current cluster nodes. Node set: %v", utils.NodeNames(readyWorkerNodes))
	}

	if !isReApply {
		lbs, err := vngcloudutil.ListLB(c.vLBSC, c.getProjectID())
		if err != nil {
			klog.Errorf("Failed to retrieve current set of load balancers: %v", err)
			return
		}
		reapplyIngress := c.trackLBUpdate.GetReapplyIngress(lbs)
		if len(reapplyIngress) > 0 {
			isReApply = true
			klog.Infof("Detected change in load balancer update tracker")
			c.trackLBUpdate = utils.NewUpdateTracker()
		}
	}

	if !isReApply {
		return
	}

	var ings *nwv1.IngressList
	// NOTE(lingxiankong): only take ingresses without ip address into consideration
	opts := apimetav1.ListOptions{}
	if ings, err = c.kubeClient.NetworkingV1().Ingresses("").List(context.TODO(), opts); err != nil {
		logrus.Errorf("Failed to retrieve current set of ingresses: %v", err)
		return
	}

	// Update each valid ingress
	for _, ing := range ings.Items {
		if !IsValid(&ing) {
			continue
		}
		copyIng := ing.DeepCopy()
		logrus.WithFields(logrus.Fields{"ingress": copyIng.Name, "namespace": copyIng.Namespace}).Debug("Starting to sync ingress")
		c.queue.AddRateLimited(Event{Obj: copyIng, Type: SyncEvent, oldObj: nil})
	}
	c.knownNodes = readyWorkerNodes
	klog.Info("Finished to handle node change.")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	obj, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(obj)
	c.addEvent(obj.(Event))
	c.queue.Forget(obj)
	return true
}

func (c *Controller) processItem(event Event) {
	klog.Infoln("EVENT:", event.Type)

	c.addUpdatingThread()
	defer c.removeUpdatingThread()

	ing := event.Obj.(*nwv1.Ingress)
	var oldIng *nwv1.Ingress
	oldIng = nil
	if event.oldObj != nil {
		oldIng = event.oldObj.(*nwv1.Ingress)
	}
	key := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)
	logger := logrus.WithFields(logrus.Fields{"ingress": key})

	switch event.Type {
	case CreateEvent:
		logger.Info("creating ingress")

		if err := c.ensureIngress(oldIng, ing); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to create vngcloud resources for ingress %s: %v", key, err))
			c.recorder.Event(ing, apiv1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to create vngcloud resources for ingress %s: %v", key, err))
		} else {
			c.recorder.Event(ing, apiv1.EventTypeNormal, "Created", fmt.Sprintf("Ingress %s", key))
		}
	case UpdateEvent:
		logger.Info("updating ingress")

		if err := c.ensureIngress(oldIng, ing); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to update vngcloud resources for ingress %s: %v", key, err))
			c.recorder.Event(ing, apiv1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to update vngcloud resources for ingress %s: %v", key, err))
		} else {
			c.recorder.Event(ing, apiv1.EventTypeNormal, "Updated", fmt.Sprintf("Ingress %s", key))
		}
	case DeleteEvent:
		logger.Info("deleting ingress")

		if err := c.deleteIngress(ing); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to delete vngcloud resources for ingress %s: %v", key, err))
			c.recorder.Event(ing, apiv1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to delete vngcloud resources for ingress %s: %v", key, err))
		} else {
			c.recorder.Event(ing, apiv1.EventTypeNormal, "Deleted", fmt.Sprintf("Ingress %s", key))
		}
	case SyncEvent:
		logger.Info("sync ingress")

		if err := c.ensureIngress(oldIng, ing); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to sync vngcloud resources for ingress %s: %v", key, err))
			c.recorder.Event(ing, apiv1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to sync vngcloud resources for ingress %s: %v", key, err))
		} else {
			c.recorder.Event(ing, apiv1.EventTypeNormal, "Synced", fmt.Sprintf("Ingress %s", key))
		}
	}

	klog.Infoln("----- DONE -----")
	// return nil
}

func (c *Controller) deleteIngress(ing *nwv1.Ingress) error {
	if option, ok := ing.Annotations[ServiceAnnotationIgnore]; ok {
		if isIgnore := utils.ParseBoolAnnotation(option, ServiceAnnotationIgnore, false); isIgnore {
			klog.Infof("Ignore ensure for service %s/%s", ing.Namespace, ing.Name)
			return nil
		}
	}

	err := c.DeleteLoadbalancer(ing)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) ensureIngress(oldIng, ing *nwv1.Ingress) error {
	if option, ok := ing.Annotations[ServiceAnnotationIgnore]; ok {
		if isIgnore := utils.ParseBoolAnnotation(option, ServiceAnnotationIgnore, false); isIgnore {
			klog.Infof("Ignore ensure for service %s/%s", ing.Namespace, ing.Name)
			return nil
		}
	}

	ingressKey := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)
	ing, err := c.updateIngressAnnotation(ingressKey)
	if err != nil {
		return err
	}

	lb, err := c.ensureCompareIngress(oldIng, ing)
	if err != nil {
		c.isReApplyNextTime = true
		return err
	}
	c.trackLBUpdate.AddUpdateTracker(lb.UUID, fmt.Sprintf("%s/%s", ing.Namespace, ing.Name), lb.UpdatedAt)
	_, err = c.updateIngressStatus(ing, lb)
	if err != nil {
		c.isReApplyNextTime = true
	}
	return err
}

// when create new ingress, you should update the load balancer name annotation immediately,
// avoid the case user update this annotation before load balancer is created
// then webhook will not allow to update this annotation (just allow when this annotation is nil)
func (c *Controller) updateIngressAnnotation(ingressKey string) (*nwv1.Ingress, error) {
	latestIngress, err := utils.GetIngress(c.ingressLister, ingressKey)
	if err != nil {
		logrus.Errorf("Failed to get the latest version of ingress %s", ingressKey)
		return nil, vErrors.ErrIngressNotFound
	}
	if latestIngress.ObjectMeta.Annotations == nil {
		latestIngress.ObjectMeta.Annotations = map[string]string{}
	}
	if _, ok := latestIngress.ObjectMeta.Annotations[ServiceAnnotationLoadBalancerName]; !ok {
		latestIngress.ObjectMeta.Annotations[ServiceAnnotationLoadBalancerName] = utils.GenerateLBName(c.getClusterID(), latestIngress.Namespace, latestIngress.Name, consts.RESOURCE_TYPE_INGRESS)
		newObj, err := c.kubeClient.NetworkingV1().Ingresses(latestIngress.Namespace).Update(context.TODO(), latestIngress, apimetav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return newObj, nil
	}
	return latestIngress, nil
}

func (c *Controller) updateIngressStatus(ing *nwv1.Ingress, lb *lObjects.LoadBalancer) (*nwv1.Ingress, error) {
	// get the latest version of ingress before update
	ingressKey := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)
	latestIngress, err := utils.GetIngress(c.ingressLister, ingressKey)
	if err != nil {
		logrus.Errorf("Failed to get the latest version of ingress %s", ingressKey)
		return nil, vErrors.ErrIngressNotFound
	}

	newIng := latestIngress.DeepCopy()
	newState := new(nwv1.IngressLoadBalancerStatus)
	newState.Ingress = []nwv1.IngressLoadBalancerIngress{{IP: lb.Address}}
	newIng.Status.LoadBalancer = *newState

	newObj, err := c.kubeClient.NetworkingV1().Ingresses(newIng.Namespace).UpdateStatus(context.TODO(), newIng, apimetav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	c.recorder.Event(ing, apiv1.EventTypeNormal, "Updated", fmt.Sprintf("Successfully associated IP address %s to ingress %s", lb.Address, newIng.Name))
	return newObj, nil
}

///////////////////////////////////////////////////////////////////
//////////////                                       //////////////
//////////////                  VKS                  //////////////
//////////////                                       //////////////
///////////////////////////////////////////////////////////////////

func (c *Controller) Init() error {
	provider, err := client.NewVContainerClient(&c.config.Global)
	if err != nil {
		klog.Errorf("failed to init VContainer client: %v", err)
		return err
	}

	provider.SetUserAgent(fmt.Sprintf(
		"vngcloud-ingress-controller/%s (ChartVersion/%s)",
		version.Version, c.config.Metadata.ChartVersion))

	c.provider = provider

	vlbSC, err := vngcloud.NewServiceClient(
		cuongpigerutils.NormalizeURL(c.getVServerURL())+"vlb-gateway/v2",
		provider, "vlb-gateway")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("failed to init VLB VNGCLOUD client")
	}
	c.vLBSC = vlbSC

	vserverSC, err := vngcloud.NewServiceClient(
		cuongpigerutils.NormalizeURL(c.getVServerURL())+"vserver-gateway/v2",
		provider, "vserver-gateway")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("failed to init VSERVER VNGCLOUD client")
	}
	c.vServerSC = vserverSC

	c.setUpPortalInfo()

	return nil
}

func (c *Controller) setUpPortalInfo() {
	c.config.Metadata = vngcloudutil.GetMetadataOption(metadata.Opts{})
	metadator := metadata.GetMetadataProvider(c.config.Metadata.SearchOrder)
	extraInfo, err := vngcloudutil.SetupPortalInfo(
		c.provider,
		metadator,
		cuongpigerutils.NormalizeURL(c.config.Global.VServerURL)+"vserver-gateway/v1")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("failed to setup portal info")
	}
	c.extraInfo = extraInfo
}

func (c *Controller) GetLoadbalancerIDByIngress(ing *nwv1.Ingress) (string, error) {
	lbsInSubnet, err := vngcloudutil.ListLB(c.vLBSC, c.getProjectID())
	if err != nil {
		klog.Errorf("error when list lb by subnet id: %v", err)
		return "", err
	}

	// check in annotation lb id
	if lbID, ok := ing.Annotations[ServiceAnnotationLoadBalancerID]; ok {
		for _, lb := range lbsInSubnet {
			if lb.UUID == lbID {
				return lb.UUID, nil
			}
		}
		klog.Infof("have annotation but not found lbID: %s", lbID)
		return "", vErrors.ErrLoadBalancerIDNotFoundAnnotation
	}

	// check in annotation lb name
	if lbName, ok := ing.Annotations[ServiceAnnotationLoadBalancerName]; ok {
		for _, lb := range lbsInSubnet {
			if lb.Name == lbName {
				return lb.UUID, nil
			}
		}
		klog.Errorf("have annotation but not found lbName: %s", lbName)
		return "", vErrors.ErrLoadBalancerNameNotFoundAnnotation
	} else {
		// check in list lb name
		lbName := utils.GenerateLBName(c.getClusterID(), ing.Namespace, ing.Name, consts.RESOURCE_TYPE_INGRESS)
		for _, lb := range lbsInSubnet {
			if lb.Name == lbName {
				klog.Infof("Found lb match Name: %s", lbName)
				return lb.UUID, nil
			}
		}
		klog.Infof("Not found lb match Name: %s", lbName)
	}

	return "", vErrors.ErrNotFound
}

func (c *Controller) DeleteLoadbalancer(ing *nwv1.Ingress) error {
	lbID, err := c.GetLoadbalancerIDByIngress(ing)
	if lbID == "" {
		klog.Infof("Not found lbID to delete")
		return nil
	}
	c.trackLBUpdate.RemoveUpdateTracker(lbID, fmt.Sprintf("%s/%s", ing.Namespace, ing.Name))
	if err != nil {
		klog.Errorln("error when ensure loadbalancer", err)
		return err
	}

	oldIngExpander, err := c.inspectIngress(ing)
	if err != nil {
		oldIngExpander, _ = c.inspectIngress(nil)
	}
	newIngExpander, err := c.inspectIngress(nil)
	if err != nil {
		klog.Errorln("error when inspect new ingress:", err)
		return err
	}

	listSecgroups, err := vngcloudutil.ListSecurityGroups(c.vServerSC, c.getProjectID())
	if err != nil {
		klog.Errorln("error when list security groups", err)
		listSecgroups = make([]*lObjects.Secgroup, 0)
	}
	defaultSecgroupName := utils.GenerateLBName(c.getClusterID(), ing.Namespace, ing.Name, consts.RESOURCE_TYPE_INGRESS)
	for _, secgroup := range listSecgroups {
		if secgroup.Name == defaultSecgroupName {
			ensureSecGroupsForInstanceDelete := func(instanceID string, secgroupsID string) error {
				// get security groups of instance
				instance, err := vngcloudutil.GetServer(c.vServerSC, c.getProjectID(), instanceID)
				if err != nil {
					klog.Errorln("error when get instance", err)
					return err
				}
				currentSecgroups := make([]string, 0)
				for _, s := range instance.SecGroups {
					currentSecgroups = append(currentSecgroups, s.Uuid)
				}
				newSecgroups, isNeedUpdate := utils.MergeStringArray(currentSecgroups, []string{secgroupsID}, []string{})
				if !isNeedUpdate {
					klog.Infof("No need to update security groups for instance: %v", instanceID)
					return nil
				}
				_, err = vngcloudutil.UpdateSecGroupsOfServer(c.vServerSC, c.getProjectID(), instanceID, newSecgroups)
				vngcloudutil.WaitForServerActive(c.vServerSC, c.getProjectID(), instanceID)
				return err
			}
			for _, instanceID := range oldIngExpander.InstanceIDs {
				err := ensureSecGroupsForInstanceDelete(instanceID, secgroup.UUID)
				if err != nil {
					klog.Errorln("error when ensure security groups for instance", err)
				}
			}
			if err = vngcloudutil.DeleteSecurityGroup(c.vServerSC, c.getProjectID(), secgroup.UUID); err != nil {
				klog.Errorln("error when delete security group", err)
			}
			break
		}
	}

	// LB should active before delete
	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)

	canDeleteAllLB := func(lbID string) bool {
		getPool, err := vngcloudutil.ListPoolOfLB(c.vLBSC, c.getProjectID(), lbID)
		if err != nil {
			klog.Errorln("error when list pool of lb", err)
			return false
		}
		if len(getPool) <= len(oldIngExpander.PoolExpander)+1 { // and default pool
			// ensure default pool have the same member
			dpool, err := vngcloudutil.FindPoolByName(c.vLBSC, c.getProjectID(), lbID, consts.DEFAULT_NAME_DEFAULT_POOL)
			if err != nil {
				if err == vErrors.ErrNotFound {
					return true
				}
				klog.Errorln("error when find default pool", err)
				return false
			}
			dpoolMembers, err := vngcloudutil.GetMemberPool(c.vLBSC, c.getProjectID(), lbID, dpool.UUID)
			if err != nil {
				klog.Errorln("error when get member of default pool", err)
				return false
			}
			if len(dpoolMembers) <= len(oldIngExpander.DefaultPool.Members) {
				return true
			}
		}
		return false
	}
	if canDeleteAllLB(lbID) {
		klog.Infof("Delete load balancer %s because it is not used with other ingress.", lbID)
		if err = vngcloudutil.DeleteLB(c.vLBSC, c.getProjectID(), lbID); err != nil {
			klog.Errorln("error when delete lb", err)
			return err
		}
		return nil
	}

	_, err = c.actionCompareIngress(lbID, oldIngExpander, newIngExpander)
	if err != nil {
		klog.Errorln("error when compare ingress", err)
		return err
	}
	return nil
}

// inspectCurrentLB inspects the current load balancer (LB) identified by lbID.
// It retrieves information about the listeners, pools, and policies associated with the LB.
// The function returns an IngressInspect struct containing the inspected data, or an error if the inspection fails.
func (c *Controller) inspectCurrentLB(lbID string) (*utils.IngressInspect, error) {
	config := NewIngressConfig(nil)

	expectPolicyName := make([]*utils.PolicyExpander, 0)
	expectPoolName := make([]*utils.PoolExpander, 0)
	expectListenerName := make([]*utils.ListenerExpander, 0)
	ingressInspect := &utils.IngressInspect{
		DefaultPool: &utils.PoolExpander{},
	}

	liss, err := vngcloudutil.ListListenerOfLB(c.vLBSC, c.getProjectID(), lbID)
	if err != nil {
		klog.Errorln("error when list listener of lb", err)
		return nil, err
	}
	for _, lis := range liss {
		listenerOpts := config.CreateListenerOptions(lis.Protocol == "HTTPS")
		listenerOpts.DefaultPoolId = &lis.DefaultPoolId
		expectListenerName = append(expectListenerName, &utils.ListenerExpander{
			UUID:       lis.UUID,
			CreateOpts: *listenerOpts,
		})
		ingressInspect.DefaultPool.PoolName = lis.DefaultPoolName
		ingressInspect.DefaultPool.UUID = lis.DefaultPoolId
	}

	getPools, err := vngcloudutil.ListPoolOfLB(c.vLBSC, c.getProjectID(), lbID)
	if err != nil {
		klog.Errorln("error when list pool of lb", err)
		return nil, err
	}
	for _, p := range getPools {
		poolMembers := make([]*pool.Member, 0)
		for _, m := range p.Members {
			poolMembers = append(poolMembers, &pool.Member{
				IpAddress:   m.Address,
				Port:        m.ProtocolPort,
				Backup:      m.Backup,
				Weight:      m.Weight,
				Name:        m.Name,
				MonitorPort: m.MonitorPort,
			})
		}
		poolOptions := config.CreatePoolOptions()
		poolOptions.PoolName = p.Name
		poolOptions.Members = poolMembers
		expectPoolName = append(expectPoolName, &utils.PoolExpander{
			UUID:       p.UUID,
			CreateOpts: *poolOptions,
		})
	}

	for _, lis := range liss {
		pols, err := vngcloudutil.ListPolicyOfListener(c.vLBSC, c.getProjectID(), lbID, lis.UUID)
		if err != nil {
			klog.Errorln("error when list policy of listener", err)
			return nil, err
		}
		for _, pol := range pols {
			l7Rules := make([]policy.Rule, 0)
			for _, r := range pol.L7Rules {
				l7Rules = append(l7Rules, policy.Rule{
					CompareType: policy.PolicyOptsCompareTypeOpt(r.CompareType),
					RuleValue:   r.RuleValue,
					RuleType:    policy.PolicyOptsRuleTypeOpt(r.RuleType),
				})
			}
			expectPolicyName = append(expectPolicyName, &utils.PolicyExpander{
				IsInUse:          false,
				ListenerID:       lis.UUID,
				UUID:             pol.UUID,
				Name:             pol.Name,
				RedirectPoolID:   pol.RedirectPoolID,
				RedirectPoolName: pol.RedirectPoolName,
				Action:           policy.PolicyOptsActionOpt(pol.Action),
				L7Rules:          l7Rules,
			})
		}
	}
	ingressInspect.PolicyExpander = expectPolicyName
	ingressInspect.PoolExpander = expectPoolName
	ingressInspect.ListenerExpander = expectListenerName
	return ingressInspect, nil
}

func (c *Controller) inspectIngress(ing *nwv1.Ingress) (*Expander, error) {
	if ing == nil {
		serviceConf := NewIngressConfig(nil)
		defaultPool := serviceConf.CreatePoolOptions()
		defaultPool.PoolName = consts.DEFAULT_NAME_DEFAULT_POOL
		defaultPool.Members = make([]*pool.Member, 0)
		return &Expander{
			serviceConf: serviceConf,
			IngressInspect: &utils.IngressInspect{
				DefaultPool:          &utils.PoolExpander{CreateOpts: *defaultPool},
				PolicyExpander:       make([]*utils.PolicyExpander, 0),
				PoolExpander:         make([]*utils.PoolExpander, 0),
				ListenerExpander:     make([]*utils.ListenerExpander, 0),
				SecGroupRuleExpander: make([]*utils.SecGroupRuleExpander, 0),
			}}, nil
	}
	serviceConf := NewIngressConfig(ing)
	ingressInspect := &utils.IngressInspect{
		Name:      ing.Name,
		Namespace: ing.Namespace,
		DefaultPool: &utils.PoolExpander{
			UUID: "",
		},
		LbOptions:            serviceConf.CreateLoadbalancerOptions(),
		PolicyExpander:       make([]*utils.PolicyExpander, 0),
		PoolExpander:         make([]*utils.PoolExpander, 0),
		ListenerExpander:     make([]*utils.ListenerExpander, 0),
		InstanceIDs:          make([]string, 0),
		SecGroupRuleExpander: make([]*utils.SecGroupRuleExpander, 0),
	}
	if ingressInspect.LbOptions.Name == "" {
		serviceConf.LoadBalancerName = utils.GenerateLBName(c.getClusterID(), ing.Namespace, ing.Name, consts.RESOURCE_TYPE_INGRESS)
		ingressInspect.LbOptions.Name = serviceConf.LoadBalancerName
	}

	nodesAfterFilter, err := utils.ListNodeWithPredicate(c.nodeLister, serviceConf.TargetNodeLabels)
	if len(nodesAfterFilter) < 1 {
		klog.Errorf("No nodes found in the cluster")
		return nil, vErrors.ErrNoNodeAvailable
	}
	if err != nil {
		klog.Errorf("Failed to retrieve current set of nodes from node lister: %v", err)
		return nil, err
	}
	membersAddr := utils.GetNodeMembersAddr(nodesAfterFilter)

	// get subnetID of this ingress
	providerIDs := utils.GetListProviderID(nodesAfterFilter)
	if len(providerIDs) < 1 {
		klog.Errorf("No nodes found in the cluster")
		return nil, vErrors.ErrNoNodeAvailable
	}
	klog.Infof("Found %d nodes for service, including of %v", len(providerIDs), providerIDs)
	servers, err := vngcloudutil.ListProviderID(c.vServerSC, c.getProjectID(), providerIDs)
	if err != nil {
		klog.Errorf("Failed to get servers from the cloud - ERROR: %v", err)
		return nil, err
	}

	// Check the nodes are in the same subnet
	subnetID, retErr := vngcloudutil.EnsureNodesInCluster(servers)
	if retErr != nil {
		klog.Errorf("All node are not in a same subnet: %v", retErr)
		return nil, retErr
	}
	networkID := vngcloudutil.GetNetworkID(servers, subnetID)
	if networkID == "" {
		klog.Errorf("Failed to get networkID from subnetID: %s", subnetID)
		return nil, vErrors.ErrNetworkIDNotFound
	}
	subnet, err := vngcloudutil.GetSubnet(c.vServerSC, c.getProjectID(), networkID, subnetID)
	if err != nil {
		klog.Errorf("Failed to get subnet: %v", err)
		return nil, err
	}
	ingressInspect.SubnetCIDR = subnet.CIDR
	ingressInspect.LbOptions.SubnetID = subnetID
	ingressInspect.InstanceIDs = providerIDs

	mapHostTLS := func(ing *nwv1.Ingress) map[string]bool {
		m := make(map[string]bool)
		for _, tls := range ing.Spec.TLS {
			for _, host := range tls.Hosts {
				m[host] = true
			}
		}
		return m
	}
	mapTLS := mapHostTLS(ing)

	GetPoolExpander := func(service *nwv1.IngressServiceBackend) (*utils.PoolExpander, error) {
		serviceName := fmt.Sprintf("%s/%s", ing.ObjectMeta.Namespace, service.Name)
		poolName := utils.GeneratePoolName(c.getClusterID(), ing.Namespace, ing.Name, consts.RESOURCE_TYPE_INGRESS,
			serviceName, int(service.Port.Number))
		nodePort, err := utils.GetServiceNodePort(c.serviceLister, serviceName, service)
		if err != nil {
			klog.Errorf("error when get node port: %v", err)
			return nil, err
		}

		monitorPort := nodePort
		if serviceConf.HealthcheckPort != 0 {
			monitorPort = serviceConf.HealthcheckPort
			if serviceConf.IsAutoCreateSecurityGroup {
				ingressInspect.AddSecgroupRule(monitorPort, secgroup_rule.CreateOptsProtocolOptTCP)
			}
		}

		members := make([]*pool.Member, 0)
		for _, addr := range membersAddr {
			members = append(members, &pool.Member{
				IpAddress:   addr,
				Port:        nodePort,
				Backup:      false,
				Weight:      1,
				Name:        poolName,
				MonitorPort: monitorPort,
			})
		}
		poolOptions := serviceConf.CreatePoolOptions()
		poolOptions.PoolName = poolName
		poolOptions.Members = members

		if serviceConf.IsAutoCreateSecurityGroup {
			ingressInspect.AddSecgroupRule(int(nodePort), secgroup_rule.CreateOptsProtocolOptTCP)
		}
		return &utils.PoolExpander{
			UUID:       "",
			CreateOpts: *poolOptions,
		}, nil
	}

	// check if have default pool
	defaultPool := serviceConf.CreatePoolOptions()
	defaultPool.PoolName = consts.DEFAULT_NAME_DEFAULT_POOL
	defaultPool.Members = make([]*pool.Member, 0)
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		defaultPoolExpander, err := GetPoolExpander(ing.Spec.DefaultBackend.Service)
		if err != nil {
			klog.Errorln("error when get default pool expander", err)
			return nil, err
		}
		defaultPool.Members = defaultPoolExpander.Members
	}
	ingressInspect.DefaultPool.CreateOpts = *defaultPool

	// ensure http listener and https listener
	if len(ing.Spec.TLS) > 0 {
		if len(serviceConf.CertificateIDs) < 1 {
			klog.Errorf("No certificate found, need to specific annotaion: %s", ServiceAnnotationCertificateIDs)
			return nil, vErrors.ErrNoCertificateFound
		} else {
			listenerHttpsOpts := serviceConf.CreateListenerOptions(true)
			listenerHttpsOpts.DefaultCertificateAuthority = &(serviceConf.CertificateIDs[0])
			if len(serviceConf.CertificateIDs) > 1 {
				listenerHttpsOpts.CertificateAuthorities = PointerOf[[]string](serviceConf.CertificateIDs[1:])
			} else {
				listenerHttpsOpts.CertificateAuthorities = PointerOf[[]string]([]string{})
			}
			listenerHttpsOpts.ClientCertificate = PointerOf[string]("")
			ingressInspect.ListenerExpander = append(ingressInspect.ListenerExpander, &utils.ListenerExpander{
				CreateOpts: *listenerHttpsOpts,
			})
		}
	}

	ingressInspect.ListenerExpander = append(ingressInspect.ListenerExpander, &utils.ListenerExpander{
		CreateOpts: *(serviceConf.CreateListenerOptions(false)),
	})

	for ruleIndex, rule := range ing.Spec.Rules {
		_, isHttpsListener := mapTLS[rule.Host]

		for pathIndex, path := range rule.HTTP.Paths {
			policyName := utils.GeneratePolicyName(c.getClusterID(), ing.Namespace, ing.Name, consts.RESOURCE_TYPE_INGRESS,
				isHttpsListener, ruleIndex, pathIndex)

			poolExpander, err := GetPoolExpander(path.Backend.Service)
			if err != nil {
				klog.Errorln("error when get pool expander:", err)
				return nil, err
			}
			ingressInspect.PoolExpander = append(ingressInspect.PoolExpander, poolExpander)

			// ensure policy
			compareType := policy.PolicyOptsCompareTypeOptEQUALS
			if path.PathType != nil && *path.PathType == nwv1.PathTypePrefix {
				compareType = policy.PolicyOptsCompareTypeOptSTARTSWITH
			}
			newRules := []policy.Rule{
				{
					RuleType:    policy.PolicyOptsRuleTypeOptPATH,
					CompareType: compareType,
					RuleValue:   path.Path,
				},
			}
			if rule.Host != "" {
				newRules = append(newRules, policy.Rule{
					RuleType:    policy.PolicyOptsRuleTypeOptHOSTNAME,
					CompareType: policy.PolicyOptsCompareTypeOptEQUALS,
					RuleValue:   rule.Host,
				})
			}
			ingressInspect.PolicyExpander = append(ingressInspect.PolicyExpander, &utils.PolicyExpander{
				IsHttpsListener:  isHttpsListener,
				IsInUse:          false,
				UUID:             "",
				Name:             policyName,
				RedirectPoolID:   "",
				RedirectPoolName: poolExpander.PoolName,
				Action:           policy.PolicyOptsActionOptREDIRECTTOPOOL,
				L7Rules:          newRules,
			})
		}
	}
	return &Expander{
		serviceConf:    serviceConf,
		IngressInspect: ingressInspect,
	}, nil
}

func (c *Controller) ensureCompareIngress(oldIng, ing *nwv1.Ingress) (*lObjects.LoadBalancer, error) {

	oldIngExpander, err := c.inspectIngress(oldIng)
	if err != nil {
		oldIngExpander, _ = c.inspectIngress(nil)
	}
	newIngExpander, err := c.inspectIngress(ing)
	if err != nil {
		klog.Errorln("error when inspect new ingress:", err)
		return nil, err
	}

	lbID, _ := c.GetLoadbalancerIDByIngress(ing)
	if lbID != "" {
		newIngExpander.serviceConf.LoadBalancerID = lbID
	}
	lbID, err = c.ensureLoadBalancerInstance(newIngExpander)
	if err != nil {
		klog.Errorln("error when ensure loadbalancer", err)
		return nil, err
	}

	lb, err := c.actionCompareIngress(lbID, oldIngExpander, newIngExpander)
	if err != nil {
		klog.Errorln("error when compare ingress", err)
		return nil, err
	}
	return lb, nil
}

// find or create lb
func (c *Controller) ensureLoadBalancerInstance(inspect *Expander) (string, error) {
	if inspect.serviceConf.LoadBalancerID == "" {
		lb, err := vngcloudutil.CreateLB(c.vLBSC, c.getProjectID(), inspect.LbOptions)
		if err != nil {
			klog.Errorf("error when create new lb: %v", err)
			return "", err
		}
		err = c.ensureTags(lb.UUID, inspect.serviceConf.Tags)
		if err != nil {
			klog.Errorln("error when ensure tags", err)
		}
		inspect.serviceConf.LoadBalancerID = lb.UUID
		lb, err = vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), inspect.serviceConf.LoadBalancerID)
		if err != nil {
			if err == vErrors.ErrLoadBalancerStatusError {
				klog.Infof("Load balancer %s is error, delete and create later", lb.UUID)
				if errr := vngcloudutil.DeleteLB(c.vLBSC, c.getProjectID(), lb.UUID); errr != nil {
					klog.Errorln("error when delete lb", err)
					return "", errr
				}
				return "", err
			}
			klog.Errorf("error when get lb: %v", err)
			return inspect.serviceConf.LoadBalancerID, err
		}
	}

	lb, err := vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), inspect.serviceConf.LoadBalancerID)
	if err != nil {
		klog.Errorf("error when get lb: %v", err)
		return inspect.serviceConf.LoadBalancerID, err
	}

	checkDetailLB := func() {
		if lb.Name != inspect.serviceConf.LoadBalancerName {
			klog.Warningf("Load balancer name (%s) not match (%s)", lb.Name, inspect.serviceConf.LoadBalancerName)
		}
		if lb.PackageID != inspect.LbOptions.PackageID {
			klog.Info("Resize load-balancer package to: ", inspect.LbOptions.PackageID)
			err := vngcloudutil.ResizeLB(c.vLBSC, c.getProjectID(), inspect.serviceConf.LoadBalancerID, inspect.LbOptions.PackageID)
			if err != nil {
				klog.Errorf("error when resize lb: %v", err)
				return
			}
			vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), inspect.serviceConf.LoadBalancerID)
		}
		if lb.Internal != (inspect.LbOptions.Scheme == loadbalancer.CreateOptsSchemeOptInternal) {
			klog.Warning("Load balancer scheme not match, must delete and recreate")
		}
	}
	checkDetailLB()
	return inspect.serviceConf.LoadBalancerID, nil
}

func (c *Controller) actionCompareIngress(lbID string, oldIngExpander, newIngExpander *Expander) (*lObjects.LoadBalancer, error) {
	var err error
	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)

	err = c.ensureTags(lbID, newIngExpander.serviceConf.Tags)
	if err != nil {
		klog.Errorln("error when ensure tags", err)
	}

	curLBExpander, err := c.inspectCurrentLB(lbID)
	if err != nil {
		klog.Errorln("error when inspect current lb", err)
		return nil, err
	}

	utils.MapIDExpander(oldIngExpander.IngressInspect, curLBExpander) // ..........................................

	var defaultPool *lObjects.Pool
	if newIngExpander.DefaultPool != nil {
		// ensure default pool
		defaultPool, err = c.ensurePool(lbID, &newIngExpander.DefaultPool.CreateOpts)
		if err != nil {
			klog.Errorln("error when ensure default pool", err)
			return nil, err
		}

		// ensure default pool member
		if oldIngExpander != nil && oldIngExpander.DefaultPool != nil && oldIngExpander.DefaultPool.Members != nil {
			_, err = c.ensureDefaultPoolMember(lbID, defaultPool.UUID, oldIngExpander.DefaultPool.Members, newIngExpander.DefaultPool.Members)
		} else {
			_, err = c.ensureDefaultPoolMember(lbID, defaultPool.UUID, nil, newIngExpander.DefaultPool.Members)
		}
		if err != nil {
			klog.Errorln("error when ensure default pool member", err)
			return nil, err
		}
	}

	// ensure all from newIngExpander
	mapPoolNameIndex := make(map[string]int)
	for poolIndex, ipool := range newIngExpander.PoolExpander {
		newPool, err := c.ensurePool(lbID, &ipool.CreateOpts)
		if err != nil {
			klog.Errorln("error when ensure pool", err)
			return nil, err
		}
		ipool.UUID = newPool.UUID
		mapPoolNameIndex[ipool.PoolName] = poolIndex
		_, err = c.ensurePoolMember(lbID, newPool.UUID, ipool.Members)
		if err != nil {
			klog.Errorln("error when ensure pool member", err)
			return nil, err
		}
	}
	mapListenerNameIndex := make(map[string]int)
	for listenerIndex, ilistener := range newIngExpander.ListenerExpander {
		ilistener.CreateOpts.DefaultPoolId = &defaultPool.UUID
		lis, err := c.ensureListener(lbID, ilistener.CreateOpts.ListenerName, ilistener.CreateOpts)
		if err != nil {
			klog.Errorln("error when ensure listener:", ilistener.CreateOpts.ListenerName, err)
			return nil, err
		}
		ilistener.UUID = lis.UUID
		mapListenerNameIndex[ilistener.CreateOpts.ListenerName] = listenerIndex
	}

	for _, ipolicy := range newIngExpander.PolicyExpander {
		// get pool name from redirect pool name
		poolIndex, isHave := mapPoolNameIndex[ipolicy.RedirectPoolName]
		if !isHave {
			klog.Errorf("pool not found in policy: %v", ipolicy.RedirectPoolName)
			return nil, err
		}
		poolID := newIngExpander.PoolExpander[poolIndex].UUID
		listenerName := consts.DEFAULT_HTTP_LISTENER_NAME
		if ipolicy.IsHttpsListener {
			listenerName = consts.DEFAULT_HTTPS_LISTENER_NAME
		}
		listenerIndex, isHave := mapListenerNameIndex[listenerName]
		if !isHave {
			klog.Errorf("listener index not found: %v", listenerName)
			return nil, err
		}
		listenerID := newIngExpander.ListenerExpander[listenerIndex].UUID
		if listenerID == "" {
			klog.Errorf("listenerID not found: %v", listenerName)
			return nil, err
		}

		policyOpts := &policy.CreateOptsBuilder{
			Name:           ipolicy.Name,
			Action:         ipolicy.Action,
			RedirectPoolID: poolID,
			Rules:          ipolicy.L7Rules,
		}
		_, err := c.ensurePolicy(lbID, listenerID, ipolicy.Name, policyOpts)
		if err != nil {
			klog.Errorln("error when ensure policy", err)
			return nil, err
		}
	}

	err = c.ensureSecurityGroups(oldIngExpander, newIngExpander)
	if err != nil {
		klog.Errorln("error when ensure security groups", err)
	}

	// delete redundant policy and pool if in oldIng
	// with id from curLBExpander
	policyWillUse := make(map[string]int)
	for policyIndex, pol := range newIngExpander.PolicyExpander {
		policyWillUse[pol.Name] = policyIndex
	}
	for _, oldIngPolicy := range oldIngExpander.PolicyExpander {
		_, isPolicyWillUse := policyWillUse[oldIngPolicy.Name]
		if !isPolicyWillUse {
			klog.Warningf("policy not in use: %v, delete", oldIngPolicy.Name)
			_, err := c.deletePolicy(lbID, oldIngPolicy.ListenerID, oldIngPolicy.Name)
			if err != nil {
				klog.Errorln("error when ensure policy", err)
				// maybe it's already deleted
				// return nil, err
			}
		}
	}

	poolWillUse := make(map[string]bool)
	for _, pool := range newIngExpander.PoolExpander {
		poolWillUse[pool.PoolName] = true
	}
	for _, oldIngPool := range oldIngExpander.PoolExpander {
		_, isPoolWillUse := poolWillUse[oldIngPool.PoolName]
		if !isPoolWillUse && oldIngPool.PoolName != consts.DEFAULT_NAME_DEFAULT_POOL {
			klog.Warningf("pool not in use: %v, delete", oldIngPool.PoolName)
			_, err := c.deletePool(lbID, oldIngPool.PoolName)
			if err != nil {
				klog.Errorln("error when ensure pool", err)
				// maybe it's already deleted
				// return nil, err
			}
		} else {
			klog.Infof("pool in use: %v, not delete", oldIngPool.PoolName)
		}
	}
	lb, _ := vngcloudutil.GetLB(c.vLBSC, c.getProjectID(), lbID)
	return lb, nil
}

func (c *Controller) ensurePool(lbID string, poolOptions *pool.CreateOpts) (*lObjects.Pool, error) {
	ipool, err := vngcloudutil.FindPoolByName(c.vLBSC, c.getProjectID(), lbID, poolOptions.PoolName)
	if err != nil {
		if err == vErrors.ErrNotFound {
			_, err := vngcloudutil.CreatePool(c.vLBSC, c.getProjectID(), lbID, poolOptions)
			if err != nil {
				klog.Errorln("error when create new pool", err)
				return nil, err
			}
			vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
			ipool, err = vngcloudutil.FindPoolByName(c.vLBSC, c.getProjectID(), lbID, poolOptions.PoolName)
			if err != nil {
				klog.Errorln("error when find pool", err)
				return nil, err
			}
		} else {
			klog.Errorln("error when find pool", err)
			return nil, err
		}
	}

	updateOptions := vngcloudutil.ComparePoolOptions(ipool, poolOptions)
	if updateOptions != nil {
		err := vngcloudutil.UpdatePool(c.vLBSC, c.getProjectID(), lbID, ipool.UUID, updateOptions)
		if err != nil {
			klog.Errorln("error when update pool", err)
			return nil, err
		}
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	}
	return ipool, nil
}

func (c *Controller) deletePool(lbID, poolName string) (*lObjects.Pool, error) {
	pool, err := vngcloudutil.FindPoolByName(c.vLBSC, c.getProjectID(), lbID, poolName)
	if err != nil {
		if err == vErrors.ErrNotFound {
			klog.Infof("pool not found: %s, maybe deleted", poolName)
			return nil, nil
		} else {
			klog.Errorln("error when find pool", err)
			return nil, err
		}
	}

	if pool.Name == consts.DEFAULT_NAME_DEFAULT_POOL {
		klog.Info("pool is default pool, not delete")
		return nil, nil
	}
	err = vngcloudutil.DeletePool(c.vLBSC, c.getProjectID(), lbID, pool.UUID)
	if err != nil {
		klog.Errorln("error when delete pool", err)
		return nil, err
	}

	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	return pool, nil
}

func (c *Controller) ensureDefaultPoolMember(lbID, poolID string, oldMembers, newMembers []*pool.Member) (*lObjects.Pool, error) {
	memsGet, err := vngcloudutil.GetMemberPool(c.vLBSC, c.getProjectID(), lbID, poolID)
	if err != nil {
		klog.Errorln("error when get pool members", err)
		return nil, err
	}
	updateMember, err := comparePoolDefaultMember(memsGet, oldMembers, newMembers)
	if err != nil {
		klog.Errorln("error when compare pool members:", err)
		return nil, err
	}
	if updateMember != nil {
		c.isReApplyNextTime = true
		err = vngcloudutil.UpdatePoolMember(c.vLBSC, c.getProjectID(), lbID, poolID, updateMember)
		if err != nil {
			klog.Errorln("error when update pool members", err)
			return nil, err
		}
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	}

	return nil, nil
}

func (c *Controller) ensurePoolMember(lbID, poolID string, members []*pool.Member) (*lObjects.Pool, error) {
	memsGet, err := vngcloudutil.GetMemberPool(c.vLBSC, c.getProjectID(), lbID, poolID)
	memsGetConvert := ConvertObjectToPoolMemberArray(memsGet)
	if err != nil {
		klog.Errorln("error when get pool members", err)
		return nil, err
	}
	if !ComparePoolMembers(members, memsGetConvert) {
		err := vngcloudutil.UpdatePoolMember(c.vLBSC, c.getProjectID(), lbID, poolID, members)
		if err != nil {
			klog.Errorln("error when update pool members", err)
			return nil, err
		}
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	}
	return nil, nil
}

func (c *Controller) ensureListener(lbID, lisName string, listenerOpts listener.CreateOpts) (*lObjects.Listener, error) {
	lis, err := vngcloudutil.FindListenerByPort(c.vLBSC, c.getProjectID(), lbID, listenerOpts.ListenerProtocolPort)
	if err != nil {
		if err == vErrors.ErrNotFound {
			// create listener point to default pool
			listenerOpts.ListenerName = lisName
			_, err := vngcloudutil.CreateListener(c.vLBSC, c.getProjectID(), lbID, &listenerOpts)
			if err != nil {
				klog.Errorln("error when create listener", err)
				return nil, err
			}
			vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
			lis, err = vngcloudutil.FindListenerByPort(c.vLBSC, c.getProjectID(), lbID, listenerOpts.ListenerProtocolPort)
			if err != nil {
				klog.Errorln("error when find listener", err)
				return nil, err
			}
		} else {
			klog.Errorln("error when find listener", err)
			return nil, err
		}
	}

	// check if listerner is right protocol
	if !strings.EqualFold(lis.Protocol, string(listenerOpts.ListenerProtocol)) {
		klog.Errorf("listener protocol not match: %s, %s", lis.Protocol, listenerOpts.ListenerProtocol)
		return nil, vErrors.ErrListenerProtocolNotMatch
	}

	updateOpts := vngcloudutil.CompareListenerOptions(lis, &listenerOpts)
	if updateOpts != nil {
		err := vngcloudutil.UpdateListener(c.vLBSC, c.getProjectID(), lbID, lis.UUID, updateOpts)
		if err != nil {
			klog.Error("error when update listener: ", err)
			return nil, err
		}
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	}

	return lis, nil
}

func (c *Controller) ensurePolicy(lbID, listenerID, policyName string, policyOpt *policy.CreateOptsBuilder) (*lObjects.Policy, error) {
	pol, err := vngcloudutil.FindPolicyByName(c.vLBSC, c.getProjectID(), lbID, listenerID, policyName)
	if err != nil {
		if err == vErrors.ErrNotFound {
			newPolicy, err := vngcloudutil.CreatePolicy(c.vLBSC, c.getProjectID(), lbID, listenerID, policyOpt)
			if err != nil {
				klog.Errorln("error when create policy", err)
				return nil, err
			}
			pol = newPolicy
			vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
		} else {
			klog.Errorln("error when find policy", err)
			return nil, err
		}
	}
	// get policy and update policy
	newpolicy, err := vngcloudutil.GetPolicy(c.vLBSC, c.getProjectID(), lbID, listenerID, pol.UUID)
	if err != nil {
		klog.Errorln("error when get policy", err)
		return nil, err
	}
	updateOpts := comparePolicy(newpolicy, policyOpt)
	if updateOpts != nil {
		err := vngcloudutil.UpdatePolicy(c.vLBSC, c.getProjectID(), lbID, listenerID, pol.UUID, updateOpts)
		if err != nil {
			klog.Errorln("error when update policy", err)
			return nil, err
		}
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	}
	return pol, nil
}

func (c *Controller) deletePolicy(lbID, listenerID, policyName string) (*lObjects.Policy, error) {
	pol, err := vngcloudutil.FindPolicyByName(c.vLBSC, c.getProjectID(), lbID, listenerID, policyName)
	if err != nil {
		if err == vErrors.ErrNotFound {
			klog.Infof("policy not found: %s, maybe deleted", policyName)
			return nil, nil
		} else {
			klog.Errorln("error when find policy", err)
			return nil, err
		}
	}
	err = vngcloudutil.DeletePolicy(c.vLBSC, c.getProjectID(), lbID, listenerID, pol.UUID)
	if err != nil {
		klog.Errorln("error when delete policy", err)
		return nil, err
	}
	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	return pol, nil
}

func (c *Controller) ensureSecurityGroups(oldInspect, inspect *Expander) error {
	if inspect.Name == "" || inspect.Namespace == "" {
		return nil
	}
	var listSecgroups []*lObjects.Secgroup
	listSecgroups, err := vngcloudutil.ListSecurityGroups(c.vServerSC, c.getProjectID())
	if err != nil {
		klog.Errorln("error when list security groups", err)
		return err
	}
	defaultSecgroupName := utils.GenerateLBName(c.getClusterID(), inspect.Namespace, inspect.Name, consts.RESOURCE_TYPE_INGRESS)
	var defaultSecgroup *lObjects.Secgroup = nil
	for _, secgroup := range listSecgroups {
		if secgroup.Name == defaultSecgroupName {
			defaultSecgroup = secgroup
		}
	}

	if inspect.serviceConf.IsAutoCreateSecurityGroup {
		if defaultSecgroup == nil {
			defaultSecgroup, err = vngcloudutil.CreateSecurityGroup(c.vServerSC, c.getProjectID(), defaultSecgroupName, "Automatically created using VNGCLOUD Ingress Controller")
			if err != nil {
				klog.Errorln("error when create security group", err)
				return err
			}
		}
		defaultSecgroup, err := vngcloudutil.GetSecurityGroup(c.vServerSC, c.getProjectID(), defaultSecgroup.UUID)
		if err != nil {
			klog.Errorln("error when get default security group", err)
			return err
		}
		ensureDefaultSecgroupRule := func() {
			// clear all inbound rules
			secgroupRules, err := vngcloudutil.ListSecurityGroupRules(c.vServerSC, c.getProjectID(), defaultSecgroup.UUID)
			if err != nil {
				klog.Errorln("error when list security group rules", err)
				return
			}

			for _, rule := range inspect.SecGroupRuleExpander {
				rule.CreateOpts.SecurityGroupID = defaultSecgroup.UUID
				rule.CreateOpts.RemoteIPPrefix = inspect.SubnetCIDR
			}

			needDelete, needCreate := vngcloudutil.CompareSecgroupRule(secgroupRules, inspect.SecGroupRuleExpander)
			for _, ruleID := range needDelete {
				err := vngcloudutil.DeleteSecurityGroupRule(c.vServerSC, c.getProjectID(), defaultSecgroup.UUID, ruleID)
				if err != nil {
					klog.Errorln("error when delete security group rule", err)
					return
				}
			}
			for _, rule := range needCreate {
				_, err := vngcloudutil.CreateSecurityGroupRule(c.vServerSC, c.getProjectID(), defaultSecgroup.UUID, &rule.CreateOpts)
				if err != nil {
					klog.Errorln("error when create security group rule", err)
					return
				}
			}
		}
		ensureDefaultSecgroupRule()
		inspect.serviceConf.SecurityGroups = append(inspect.serviceConf.SecurityGroups, defaultSecgroup.UUID)
	}
	if len(inspect.serviceConf.SecurityGroups) < 1 || len(inspect.InstanceIDs) < 1 {
		return nil
	}
	// add default security group to old inspect
	if oldInspect != nil && oldInspect.serviceConf.IsAutoCreateSecurityGroup && defaultSecgroup != nil {
		oldInspect.serviceConf.SecurityGroups = append(oldInspect.serviceConf.SecurityGroups, defaultSecgroup.UUID)
	}

	listSecgroups, err = vngcloudutil.ListSecurityGroups(c.vServerSC, c.getProjectID())
	if err != nil {
		klog.Errorln("error when list security groups", err)
		return err
	}

	// validate security groups
	validSecgroups := make([]string, 0)
	mapSecgroups := make(map[string]bool)
	for _, secgroup := range listSecgroups {
		mapSecgroups[secgroup.UUID] = true
	}
	for _, secgroup := range inspect.serviceConf.SecurityGroups {
		if _, isHave := mapSecgroups[secgroup]; !isHave {
			klog.Errorf("security group not found: %v", secgroup)
		} else {
			validSecgroups = append(validSecgroups, secgroup)
		}
	}

	ensureSecGroupsForInstance := func(instanceID string, oldSecgroups, secgroups []string) error {
		// get security groups of instance
		instance, err := vngcloudutil.GetServer(c.vServerSC, c.getProjectID(), instanceID)
		if err != nil {
			klog.Errorln("error when get instance", err)
			return err
		}
		currentSecgroups := make([]string, 0)
		for _, secgroup := range instance.SecGroups {
			currentSecgroups = append(currentSecgroups, secgroup.Uuid)
		}
		newSecgroups, isNeedUpdate := utils.MergeStringArray(currentSecgroups, oldSecgroups, secgroups)
		if !isNeedUpdate {
			klog.Infof("No need to update security groups for instance: %v", instanceID)
			return nil
		}
		_, err = vngcloudutil.UpdateSecGroupsOfServer(c.vServerSC, c.getProjectID(), instanceID, newSecgroups)
		vngcloudutil.WaitForServerActive(c.vServerSC, c.getProjectID(), instanceID)
		return err
	}
	for _, instanceID := range inspect.InstanceIDs {
		err := ensureSecGroupsForInstance(instanceID, oldInspect.serviceConf.SecurityGroups, validSecgroups)
		if err != nil {
			klog.Errorln("error when ensure security groups for instance", err)
		}
	}
	return nil
}

func (c *Controller) ensureTags(lbID string, tags map[string]string) error {
	// get tags of lb
	getTags, err := vngcloudutil.GetTags(c.vServerSC, c.getProjectID(), lbID)
	if err != nil {
		klog.Errorln("error when get tags", err)
		return err
	}
	// merge tags
	tagMap := make(map[string]string)
	for _, tag := range getTags {
		tagMap[tag.Key] = tag.Value
	}
	isNeedUpdate := false
	for key, value := range tags {
		if tagMap[key] != value {
			isNeedUpdate = true
			tagMap[key] = value
		}
	}
	vksClusterTags := tagMap[consts.VKS_TAG_KEY]
	newTags := vngcloudutil.JoinVKSTag(vksClusterTags, c.getClusterID())
	if newTags != vksClusterTags {
		isNeedUpdate = true
		tagMap[consts.VKS_TAG_KEY] = newTags
	}
	if !isNeedUpdate {
		klog.Infof("No need to update tags for lb: %v", lbID)
		return nil
	}
	// update tags
	err = vngcloudutil.UpdateTags(c.vServerSC, c.getProjectID(), lbID, tagMap)
	return err
}

func (c *Controller) getProjectID() string {
	return c.extraInfo.ProjectID
}

// NAME RESOURCE
func (c *Controller) getClusterName() string {
	return c.config.Cluster.ClusterName
}

func (c *Controller) getClusterID() string {
	return c.config.Cluster.ClusterID
}

func (s *Controller) getVServerURL() string {
	return s.config.Global.VServerURL
}
