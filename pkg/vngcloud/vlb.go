package vngcloud

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	lSdkClient "github.com/vngcloud/cloud-provider-vngcloud/pkg/client"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	lMetrics "github.com/vngcloud/cloud-provider-vngcloud/pkg/metrics"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
	vErrors "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/errors"
	lMetadata "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/metadata"
	vngcloudutil "github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/vngcloud"
	lClient "github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	lCoreV1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

type Expander struct {
	serviceConf *ServiceConfig
	*utils.IngressInspect
}
type (
	// VLbOpts is default vLB configurations that are loaded from the vcontainer-ccm config file
	VLbOpts struct {
		DefaultL4PackageID               string `gcfg:"default-l4-package-id"`
		DefaultListenerAllowedCIDRs      string `gcfg:"default-listener-allowed-cidrs"`
		DefaultIdleTimeoutClient         int    `gcfg:"default-idle-timeout-client"`
		DefaultIdleTimeoutMember         int    `gcfg:"default-idle-timeout-member"`
		DefaultIdleTimeoutConnection     int    `gcfg:"default-idle-timeout-connection"`
		DefaultPoolAlgorithm             string `gcfg:"default-pool-algorithm"`
		DefaultMonitorHealthyThreshold   int    `gcfg:"default-monitor-healthy-threshold"`
		DefaultMonitorUnhealthyThreshold int    `gcfg:"default-monitor-unhealthy-threshold"`
		DefaultMonitorTimeout            int    `gcfg:"default-monitor-timeout"`
		DefaultMonitorInterval           int    `gcfg:"default-monitor-interval"`
		DefaultMonitorHttpMethod         string `gcfg:"default-monitor-http-method"`
		DefaultMonitorHttpPath           string `gcfg:"default-monitor-http-path"`
		DefaultMonitorHttpSuccessCode    string `gcfg:"default-monitor-http-success-code"`
		DefaultMonitorHttpVersion        string `gcfg:"default-monitor-http-version"`
		DefaultMonitorHttpDomainName     string `gcfg:"default-monitor-http-domain-name"`
		DefaultMonitorProtocol           string `gcfg:"default-monitor-protocol"`
	}

	// vLB is the implementation of the VNG CLOUD for actions on load balancer
	vLB struct {
		vLBSC     *lClient.ServiceClient
		vServerSC *lClient.ServiceClient

		kubeClient    kubernetes.Interface
		eventRecorder record.EventRecorder
		vLbConfig     VLbOpts
		extraInfo     *vngcloudutil.ExtraInfo
		trackLBUpdate *utils.UpdateTracker
		serviceCache  map[string]*lCoreV1.Service

		serviceLister       corelisters.ServiceLister
		serviceListerSynced cache.InformerSynced
		nodeLister          corelisters.NodeLister
		nodeListerSynced    cache.InformerSynced
		stopCh              chan struct{}
		informer            informers.SharedInformerFactory
		mu                  sync.Mutex
		numOfUpdatingThread int
		config              *Config
		isReApplyNextTime   bool
	}

	// Config is the configuration for the VNG CLOUD load balancer controller,
	// it is loaded from the vcontainer-ccm config file
	Config struct {
		Global   lSdkClient.AuthOpts // global configurations, it is loaded from Helm helpers and values.yaml
		VLB      VLbOpts             // vLB configurations, it is loaded from Helm helpers and values.yaml
		Metadata lMetadata.Opts      // metadata service config, by default is empty
		Cluster  struct {
			ClusterName string `gcfg:"cluster-name"`
			ClusterID   string `gcfg:"cluster-id"`
		}
	}
)

func (c *vLB) Init() {
	kubeInformerFactory := informers.NewSharedInformerFactory(c.kubeClient, time.Second*30)
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	c.nodeLister = nodeInformer.Lister()
	c.nodeListerSynced = nodeInformer.Informer().HasSynced
	c.serviceLister = serviceInformer.Lister()
	c.serviceListerSynced = serviceInformer.Informer().HasSynced
	c.stopCh = make(chan struct{})
	c.informer = kubeInformerFactory
	c.numOfUpdatingThread = 0

	defer close(c.stopCh)
	go c.informer.Start(c.stopCh)

	// wait for the caches to synchronize before starting the worker
	if !cache.WaitForCacheSync(c.stopCh, c.serviceListerSynced, c.nodeListerSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	go wait.Until(c.nodeSyncLoop, 60*time.Second, c.stopCh)
	<-c.stopCh
}

// ****************************** IMPLEMENTATIONS OF KUBERNETES CLOUD PROVIDER INTERFACE *******************************

func (c *vLB) GetLoadBalancer(pCtx context.Context, clusterName string, pService *lCoreV1.Service) (*lCoreV1.LoadBalancerStatus, bool, error) {
	mc := lMetrics.NewMetricContext("loadbalancer", "ensure")
	klog.InfoS("GetLoadBalancer", "cluster", clusterName, "service", klog.KObj(pService))
	status, existed, err := c.ensureGetLoadBalancer(pCtx, clusterName, pService)
	return status, existed, mc.ObserveReconcile(err)
}

func (c *vLB) GetLoadBalancerName(_ context.Context, clusterName string, pService *lCoreV1.Service) string {
	return utils.GenerateLBName(c.getClusterID(), pService.Namespace, pService.Name, consts.RESOURCE_TYPE_SERVICE)
}

func (c *vLB) EnsureLoadBalancer(
	pCtx context.Context, clusterName string, pService *lCoreV1.Service, pNodes []*lCoreV1.Node) (*lCoreV1.LoadBalancerStatus, error) {
	c.addUpdatingThread()
	defer c.removeUpdatingThread()
	mc := lMetrics.NewMetricContext("loadbalancer", "ensure")
	klog.InfoS("EnsureLoadBalancer", "cluster", clusterName, "service", klog.KObj(pService))
	status, err := c.ensureLoadBalancer(pCtx, clusterName, pService, pNodes)
	if err != nil {
		c.isReApplyNextTime = true
	}
	return status, mc.ObserveReconcile(err)
}

// UpdateLoadBalancer updates hosts under the specified load balancer. This will be executed when user add or remove nodes
// from the cluster
func (c *vLB) UpdateLoadBalancer(pCtx context.Context, clusterName string, pService *lCoreV1.Service, pNodes []*lCoreV1.Node) error {
	nodeNames := make([]string, 0, len(pNodes))
	for _, node := range pNodes {
		nodeNames = append(nodeNames, node.Name)
	}
	klog.Infof("UpdateLoadBalancer: update load balancer for service %s/%s, the nodes are: %v",
		pService.Namespace, pService.Name, nodeNames)
	c.addUpdatingThread()
	defer c.removeUpdatingThread()
	mc := lMetrics.NewMetricContext("loadbalancer", "update-loadbalancer")
	klog.InfoS("UpdateLoadBalancer", "cluster", clusterName, "service", klog.KObj(pService))
	_, err := c.ensureLoadBalancer(pCtx, clusterName, pService, pNodes)
	if err != nil {
		c.isReApplyNextTime = true
	}
	return mc.ObserveReconcile(err)
}

func (c *vLB) EnsureLoadBalancerDeleted(pCtx context.Context, clusterName string, pService *lCoreV1.Service) error {
	c.addUpdatingThread()
	defer c.removeUpdatingThread()
	mc := lMetrics.NewMetricContext("loadbalancer", "ensure")
	klog.InfoS("EnsureLoadBalancerDeleted", "cluster", clusterName, "service", klog.KObj(pService))
	err := c.ensureDeleteLoadBalancer(pCtx, clusterName, pService)
	return mc.ObserveReconcile(err)
}

// ************************************************** PRIVATE METHODS **************************************************

func (c *vLB) ensureLoadBalancer(
	pCtx context.Context, clusterName string, pService *lCoreV1.Service, pNodes []*lCoreV1.Node) ( // params
	rLb *lCoreV1.LoadBalancerStatus, rErr error) { // returns

	if option, ok := pService.Annotations[ServiceAnnotationIgnore]; ok {
		if isIgnore := utils.ParseBoolAnnotation(option, ServiceAnnotationIgnore, false); isIgnore {
			klog.Infof("Ignore ensure for service %s/%s", pService.Namespace, pService.Name)
			return nil, nil
		}
	}

	// Patcher the service to prevent the service is updated by other controller
	patcher := newServicePatcher(c.kubeClient, pService)
	defer func() {
		rErr = patcher.Patch(pCtx, rErr)
	}()

	serviceKey := fmt.Sprintf("%s/%s", pService.Namespace, pService.Name)
	oldIngExpander, _ := c.inspectService(nil)
	if oldService, ok := c.serviceCache[serviceKey]; ok {
		oldIngExpander, _ = c.inspectService(oldService)
	}
	newIngExpander, err := c.inspectService(pService)
	if err != nil {
		klog.Errorln("error when inspect new ingress:", err)
		return nil, err
	}

	lbID, _ := c.GetLoadbalancerIDByService(pService)
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

	klog.V(5).Infof("processing load balancer status")
	lbStatus := c.createLoadBalancerStatus(pService, lb)

	userLb, _ := vngcloudutil.GetLB(c.vLBSC, c.getProjectID(), lb.UUID)
	c.trackLBUpdate.AddUpdateTracker(userLb.UUID, fmt.Sprintf("%s/%s", pService.Namespace, pService.Name), userLb.UpdatedAt)
	c.serviceCache[serviceKey] = pService.DeepCopy()

	klog.Infof(
		"Load balancer %s for service %s/%s is ready to use for Kubernetes controller\n----- DONE ----- ",
		lb.Name, pService.Namespace, pService.Name)
	return lbStatus, nil
}

func (c *vLB) createLoadBalancerStatus(pService *lCoreV1.Service, lb *lObjects.LoadBalancer) *lCoreV1.LoadBalancerStatus {
	if pService.ObjectMeta.Annotations == nil {
		pService.ObjectMeta.Annotations = map[string]string{}
	}
	// pService.ObjectMeta.Annotations[ServiceAnnotationLoadBalancerID] = lb.UUID
	pService.ObjectMeta.Annotations[ServiceAnnotationLoadBalancerName] = lb.Name

	status := &lCoreV1.LoadBalancerStatus{}
	// Default to IP
	status.Ingress = []lCoreV1.LoadBalancerIngress{{IP: lb.Address}}
	return status
}

func (c *vLB) getProjectID() string {
	return c.extraInfo.ProjectID
}

func (c *vLB) ensureDeleteLoadBalancer(pCtx context.Context, clusterName string, pService *lCoreV1.Service) error {
	if option, ok := pService.Annotations[ServiceAnnotationIgnore]; ok {
		if isIgnore := utils.ParseBoolAnnotation(option, ServiceAnnotationIgnore, false); isIgnore {
			klog.Infof("Ignore ensure for service %s/%s", pService.Namespace, pService.Name)
			return nil
		}
	}

	lbID, err := c.GetLoadbalancerIDByService(pService)
	if lbID == "" {
		klog.Infof("Not found lbID to delete")
		return nil
	}
	c.trackLBUpdate.RemoveUpdateTracker(lbID, fmt.Sprintf("%s/%s", pService.Namespace, pService.Name))
	if err != nil {
		klog.Errorln("error when ensure loadbalancer", err)
		return err
	}

	oldIngExpander, err := c.inspectService(pService)
	if err != nil {
		oldIngExpander, _ = c.inspectService(nil)
	}
	newIngExpander, err := c.inspectService(nil)
	if err != nil {
		klog.Errorln("error when inspect new service:", err)
		return err
	}

	listSecgroups, err := vngcloudutil.ListSecurityGroups(c.vServerSC, c.getProjectID())
	if err != nil {
		klog.Errorln("error when list security groups", err)
		listSecgroups = make([]*lObjects.Secgroup, 0)
	}
	defaultSecgroupName := utils.GenerateLBName(c.getClusterID(), pService.Namespace, pService.Name, consts.RESOURCE_TYPE_SERVICE)
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

	canDeleteAllLB := func(lbID string) bool {
		getPool, err := vngcloudutil.ListPoolOfLB(c.vLBSC, c.getProjectID(), lbID)
		if err != nil {
			klog.Errorln("error when list pool of lb", err)
			return false
		}
		getListener, err := vngcloudutil.ListListenerOfLB(c.vLBSC, c.getProjectID(), lbID)
		if err != nil {
			klog.Errorln("error when list listener of lb", err)
			return false
		}
		if len(getPool) <= len(oldIngExpander.PoolExpander) && len(getListener) <= len(oldIngExpander.ListenerExpander) {
			return true
		}
		return false
	}
	if canDeleteAllLB(lbID) {
		klog.Infof("Delete load balancer %s because it is not used with other service.", lbID)
		if err = vngcloudutil.DeleteLB(c.vLBSC, c.getProjectID(), lbID); err != nil {
			klog.Errorln("error when delete lb", err)
			return err
		}
		return nil
	}

	_, err = c.actionCompareIngress(lbID, oldIngExpander, newIngExpander)
	if err != nil {
		klog.Errorln("error when compare service", err)
		return err
	}
	return nil
}

func (c *vLB) ensureGetLoadBalancer(pCtx context.Context, clusterName string, pService *lCoreV1.Service) (*lCoreV1.LoadBalancerStatus, bool, error) {
	lbID, _ := c.GetLoadbalancerIDByService(pService)
	if lbID == "" {
		klog.Infof("Load balancer is not existed")
		return nil, false, nil
	}

	lb, err := vngcloudutil.GetLB(c.vLBSC, c.getProjectID(), lbID)
	if err != nil {
		klog.Errorf("error when get lb: %v", err)
		return nil, false, nil
	}

	lbStatus := c.createLoadBalancerStatus(pService, lb)
	return lbStatus, true, nil
}

// ********************************************* DIRECTLY SUPPORT FUNCTIONS ********************************************
func (c *vLB) addUpdatingThread() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.numOfUpdatingThread++
}

func (c *vLB) removeUpdatingThread() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.numOfUpdatingThread--
}

func (c *vLB) nodeSyncLoop() {
	klog.Infoln("------------ nodeSyncLoop() ------------")
	if c.numOfUpdatingThread > 0 {
		klog.Infof("Skip nodeSyncLoop() because the controller is in the update mode.")
		return
	}
	isReApply := false
	if c.isReApplyNextTime {
		c.isReApplyNextTime = false
		isReApply = true
	}

	if !isReApply {
		lbs, err := vngcloudutil.ListLB(c.vLBSC, c.getProjectID())
		if err != nil {
			klog.Errorf("failed to find load balancers for cluster %s: %v", c.getClusterName(), err)
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

	readyWorkerNodes, err := utils.ListNodeWithPredicate(c.nodeLister, make(map[string]string, 0))
	if err != nil {
		klog.Errorf("Failed to retrieve current set of nodes from node lister: %v", err)
		c.isReApplyNextTime = true
		return
	}

	services, err := utils.ListServiceWithPredicate(c.serviceLister)
	if err != nil {
		klog.Errorf("Failed to retrieve current set of services from service lister: %v", err)
		c.isReApplyNextTime = true
		return
	}

	for _, service := range services {
		if _, err := c.EnsureLoadBalancer(context.Background(), c.getClusterName(), service, readyWorkerNodes); err != nil {
			klog.Errorf("Failed to reapply load balancer for service %s: %v", service.Name, err)
		}
	}
}

func (c *vLB) getClusterName() string {
	return c.config.Cluster.ClusterName
}
func (c *vLB) getClusterID() string {
	return c.config.Cluster.ClusterID
}

func (c *vLB) inspectService(pService *lCoreV1.Service) (*Expander, error) {
	if pService == nil {
		return &Expander{
			serviceConf: NewServiceConfig(nil),
			IngressInspect: &utils.IngressInspect{
				DefaultPool:          nil,
				PolicyExpander:       make([]*utils.PolicyExpander, 0),
				PoolExpander:         make([]*utils.PoolExpander, 0),
				ListenerExpander:     make([]*utils.ListenerExpander, 0),
				SecurityGroups:       make([]string, 0),
				SecGroupRuleExpander: make([]*utils.SecGroupRuleExpander, 0),
			},
		}, nil
	}

	plog := logrus.WithContext(context.Background()).WithFields(map[string]interface{}{
		"Service": pService.Name,
	})

	// Check if the service spec has any port, if not, return error
	ports := pService.Spec.Ports
	if len(ports) <= 0 {
		return nil, vErrors.NewErrServicePortEmpty()
	}

	serviceConf := NewServiceConfig(pService)

	ingressInspect := &utils.IngressInspect{
		Name:                 pService.Name,
		Namespace:            pService.Namespace,
		DefaultPool:          nil,
		LbOptions:            serviceConf.CreateLoadbalancerOptions(),
		PolicyExpander:       make([]*utils.PolicyExpander, 0),
		PoolExpander:         make([]*utils.PoolExpander, 0),
		ListenerExpander:     make([]*utils.ListenerExpander, 0),
		SecurityGroups:       make([]string, 0),
		InstanceIDs:          make([]string, 0),
		SecGroupRuleExpander: make([]*utils.SecGroupRuleExpander, 0),
	}
	if ingressInspect.LbOptions.Name == "" {
		serviceConf.LoadBalancerName = utils.GenerateLBName(c.getClusterID(), pService.Namespace, pService.Name, consts.RESOURCE_TYPE_SERVICE)
		ingressInspect.LbOptions.Name = serviceConf.LoadBalancerName
	}

	nodeObjs, err := utils.ListNodeWithPredicate(c.nodeLister, serviceConf.TargetNodeLabels)
	if len(nodeObjs) < 1 {
		logrus.Errorf("No nodes found in the cluster")
		return nil, vErrors.ErrNoNodeAvailable
	}
	if err != nil {
		plog.Errorf("Failed to retrieve current set of nodes from node lister: %v", err)
		return nil, err
	}
	membersAddr := utils.GetNodeMembersAddr(nodeObjs)

	// get subnetID of this ingress
	providerIDs := utils.GetListProviderID(nodeObjs)
	if len(providerIDs) < 1 {
		plog.Errorf("No nodes found in the cluster")
		return nil, vErrors.ErrNoNodeAvailable
	}
	plog.Infof("Found %d nodes for service, including of %v", len(providerIDs), providerIDs)
	servers, err := vngcloudutil.ListProviderID(c.vServerSC, c.getProjectID(), providerIDs)
	if err != nil {
		plog.Errorf("Failed to get servers from the cloud - ERROR: %v", err)
		return nil, err
	}

	// Check the nodes are in the same subnet
	subnetID, retErr := vngcloudutil.EnsureNodesInCluster(servers)
	if retErr != nil {
		plog.Errorf("All node are not in a same subnet: %v", retErr)
		return nil, retErr
	}
	if option, ok := pService.Annotations[ServiceAnnotationInboundCIDRs]; ok {
		ingressInspect.AllowCIDR = option
	} else {
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
		ingressInspect.AllowCIDR = subnet.CIDR
	}
	ingressInspect.LbOptions.SubnetID = subnetID
	ingressInspect.InstanceIDs = providerIDs

	// Ensure pools and listener for this loadbalancer
	for _, port := range pService.Spec.Ports {
		poolName := serviceConf.GenPoolName(c.getClusterID(), pService, consts.RESOURCE_TYPE_SERVICE, port)
		listenerName := serviceConf.GenListenerName(c.getClusterID(), pService, consts.RESOURCE_TYPE_SERVICE, port)

		monitorPort := int(port.NodePort)
		if serviceConf.HealthcheckPort != 0 {
			monitorPort = serviceConf.HealthcheckPort
			if serviceConf.IsAutoCreateSecurityGroup {
				ingressInspect.AddSecgroupRule(monitorPort,
					vngcloudutil.HealthcheckProtocoToSecGroupProtocol(string(port.Protocol)))
				if strings.EqualFold(string(port.Protocol), "UDP") {
					ingressInspect.AddSecgroupRule(monitorPort,
						vngcloudutil.HealthcheckProtocoToSecGroupProtocol("ICMP"))
				}
			}
		}

		members := make([]*pool.Member, 0)
		for _, addr := range membersAddr {
			members = append(members, &pool.Member{
				IpAddress:   addr,
				Port:        int(port.NodePort),
				Backup:      false,
				Weight:      1,
				Name:        poolName,
				MonitorPort: monitorPort,
			})
		}
		poolOptions := serviceConf.CreatePoolOptions(port)
		poolOptions.PoolName = poolName
		poolOptions.Members = members

		if serviceConf.IsAutoCreateSecurityGroup {
			if serviceConf.IsAutoCreateSecurityGroup {
				ingressInspect.AddSecgroupRule(int(port.NodePort),
					vngcloudutil.HealthcheckProtocoToSecGroupProtocol(string(port.Protocol)))
				if strings.EqualFold(string(port.Protocol), "UDP") {
					ingressInspect.AddSecgroupRule(monitorPort,
						vngcloudutil.HealthcheckProtocoToSecGroupProtocol("ICMP"))
				}
			}
		}

		listenerOptions := serviceConf.CreateListenerOptions(port)
		listenerOptions.ListenerName = listenerName

		ingressInspect.PoolExpander = append(ingressInspect.PoolExpander, &utils.PoolExpander{
			UUID:       "",
			CreateOpts: *poolOptions,
		})
		ingressInspect.ListenerExpander = append(ingressInspect.ListenerExpander, &utils.ListenerExpander{
			DefaultPoolName: poolName,
			CreateOpts:      *listenerOptions,
		})
	}
	return &Expander{
		serviceConf:    serviceConf,
		IngressInspect: ingressInspect,
	}, nil
}

func (c *vLB) ensureLoadBalancerInstance(inspect *Expander) (string, error) {
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
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), inspect.serviceConf.LoadBalancerID)
	}

	lb, err := vngcloudutil.GetLB(c.vLBSC, c.getProjectID(), inspect.serviceConf.LoadBalancerID)
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

func (c *vLB) GetLoadbalancerIDByService(pService *lCoreV1.Service) (string, error) {
	lbsInSubnet, err := vngcloudutil.ListLB(c.vLBSC, c.getProjectID())
	if err != nil {
		klog.Errorf("error when list lb by subnet id: %v", err)
		return "", err
	}

	// check in annotation lb id
	if lbID, ok := pService.Annotations[ServiceAnnotationLoadBalancerID]; ok {
		for _, lb := range lbsInSubnet {
			if lb.UUID == lbID {
				return lb.UUID, nil
			}
		}
		klog.Infof("have annotation but not found lbID: %s", lbID)
		return "", vErrors.ErrLoadBalancerIDNotFoundAnnotation
	}

	// check in annotation lb name
	if lbName, ok := pService.Annotations[ServiceAnnotationLoadBalancerName]; ok {
		for _, lb := range lbsInSubnet {
			if lb.Name == lbName {
				return lb.UUID, nil
			}
		}
		klog.Errorf("have annotation but not found lbName: %s", lbName)
		return "", vErrors.ErrLoadBalancerNameNotFoundAnnotation
	} else {
		// check in list lb name
		lbName := utils.GenerateLBName(c.getClusterID(), pService.Namespace, pService.Name, consts.RESOURCE_TYPE_SERVICE)
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

func (c *vLB) actionCompareIngress(lbID string, oldIngExpander, newIngExpander *Expander) (*lObjects.LoadBalancer, error) {
	var err error
	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)

	err = c.ensureTags(lbID, newIngExpander.serviceConf.Tags)
	if err != nil {
		klog.Errorln("error when ensure tags", err)
	}

	// ensure all from newIngExpander
	mapPoolNameIndex := make(map[string]int)
	for poolIndex, ipool := range newIngExpander.PoolExpander {
		newPool, err := c.ensurePoolV2(lbID, &ipool.CreateOpts)
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
		poolIndex, isHave := mapPoolNameIndex[ilistener.DefaultPoolName]
		if !isHave {
			klog.Errorf("pool not found in policy: %v", ilistener.DefaultPoolName)
			return nil, err
		}
		ilistener.CreateOpts.DefaultPoolId = PointerOf(newIngExpander.PoolExpander[poolIndex].UUID)

		lis, err := c.ensureListenerV2(lbID, ilistener.CreateOpts.ListenerName, ilistener.CreateOpts)
		if err != nil {
			klog.Errorln("error when ensure listener:", ilistener.CreateOpts.ListenerName, err)
			return nil, err
		}
		ilistener.UUID = lis.UUID
		mapListenerNameIndex[ilistener.CreateOpts.ListenerName] = listenerIndex
	}

	err = c.ensureSecurityGroups(oldIngExpander, newIngExpander)
	if err != nil {
		klog.Errorln("error when ensure security groups", err)
	}

	// delete redundant policy and pool if in oldIng
	// with id from curLBExpander
	listenerWillUse := make(map[string]int)
	for lIndex, lis := range newIngExpander.ListenerExpander {
		listenerWillUse[lis.ListenerName] = lIndex
	}
	for _, oListener := range oldIngExpander.ListenerExpander {
		_, isInUse := listenerWillUse[oListener.ListenerName]
		if !isInUse {
			klog.Warningf("listener not in use: %v, delete", oListener.ListenerName)
			_, err := c.deleteListener(lbID, oListener.ListenerName)
			if err != nil {
				klog.Errorln("error when ensure listener", err)
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

func (c *vLB) ensurePoolV2(lbID string, poolOptions *pool.CreateOpts) (*lObjects.Pool, error) {
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
func (c *vLB) deletePool(lbID, poolName string) (*lObjects.Pool, error) {
	iPool, err := vngcloudutil.FindPoolByName(c.vLBSC, c.getProjectID(), lbID, poolName)
	if err != nil {
		if err == vErrors.ErrNotFound {
			klog.Infof("pool not found: %s, maybe deleted", poolName)
			return nil, nil
		} else {
			klog.Errorln("error when find pool", err)
			return nil, err
		}
	}

	err = vngcloudutil.DeletePool(c.vLBSC, c.getProjectID(), lbID, iPool.UUID)
	if err != nil {
		klog.Errorln("error when delete pool", err)
		return nil, err
	}

	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	return iPool, nil
}

func (c *vLB) ensurePoolMember(lbID, poolID string, members []*pool.Member) (*lObjects.Pool, error) {
	memsGet, err := vngcloudutil.GetMemberPool(c.vLBSC, c.getProjectID(), lbID, poolID)
	memsGetConvert := vngcloudutil.ConvertObjectToPoolMemberArray(memsGet)
	if err != nil {
		klog.Errorln("error when get pool members", err)
		return nil, err
	}
	if !vngcloudutil.ComparePoolMembers(members, memsGetConvert) {
		err := vngcloudutil.UpdatePoolMember(c.vLBSC, c.getProjectID(), lbID, poolID, members)
		if err != nil {
			klog.Errorln("error when update pool members", err)
			return nil, err
		}
		vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	}
	return nil, nil
}

func (c *vLB) ensureListenerV2(lbID, lisName string, listenerOpts listener.CreateOpts) (*lObjects.Listener, error) {
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

func (c *vLB) deleteListener(lbID, listenerName string) (*lObjects.Listener, error) {
	ilistener, err := vngcloudutil.FindListenerByName(c.vLBSC, c.getProjectID(), lbID, listenerName)
	if err != nil {
		if err == vErrors.ErrNotFound {
			klog.Infof("listener not found: %s, maybe deleted", listenerName)
			return nil, nil
		} else {
			klog.Errorln("error when find listener to delete:", err)
			return nil, err
		}
	}
	err = vngcloudutil.DeleteListener(c.vLBSC, c.getProjectID(), lbID, ilistener.UUID)
	if err != nil {
		klog.Errorln("error when delete listener", err)
		return nil, err
	}
	vngcloudutil.WaitForLBActive(c.vLBSC, c.getProjectID(), lbID)
	return ilistener, nil
}

func (c *vLB) ensureSecurityGroups(oldInspect, inspect *Expander) error {
	if inspect.Name == "" || inspect.Namespace == "" {
		return nil
	}
	var listSecgroups []*lObjects.Secgroup
	listSecgroups, err := vngcloudutil.ListSecurityGroups(c.vServerSC, c.getProjectID())
	if err != nil {
		klog.Errorln("error when list security groups", err)
		return err
	}
	defaultSecgroupName := utils.GenerateLBName(c.getClusterID(), inspect.Namespace, inspect.Name, consts.RESOURCE_TYPE_SERVICE)
	var defaultSecgroup *lObjects.Secgroup = nil
	for _, secgroup := range listSecgroups {
		if secgroup.Name == defaultSecgroupName {
			defaultSecgroup = secgroup
		}
	}

	if inspect.serviceConf.IsAutoCreateSecurityGroup {
		if defaultSecgroup == nil {
			defaultSecgroup, err = vngcloudutil.CreateSecurityGroup(c.vServerSC, c.getProjectID(), defaultSecgroupName, "Automatically created using VNGCLOUD Controller Manager")
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
				rule.CreateOpts.RemoteIPPrefix = inspect.AllowCIDR
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
		inspect.SecurityGroups = []string{defaultSecgroup.UUID}
	}
	if len(inspect.SecurityGroups) < 1 || len(inspect.InstanceIDs) < 1 {
		return nil
	}

	// add default security group to old inspect
	if oldInspect != nil && oldInspect.serviceConf.IsAutoCreateSecurityGroup && defaultSecgroup != nil {
		oldInspect.SecurityGroups = append(oldInspect.SecurityGroups, defaultSecgroup.UUID)
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
	for _, secgroup := range inspect.SecurityGroups {
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
		err := ensureSecGroupsForInstance(instanceID, oldInspect.SecurityGroups, validSecgroups)
		if err != nil {
			klog.Errorln("error when ensure security groups for instance", err)
		}
	}
	return nil
}

func (c *vLB) ensureTags(lbID string, tags map[string]string) error {
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
