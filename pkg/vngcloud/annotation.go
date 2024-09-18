package vngcloud

import (
	"fmt"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX = "vks.vngcloud.vn"
)

type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP       TargetType = "ip"
)

// Annotations
const (
	ServiceAnnotationIgnore = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/ignore"

	// ServiceAnnotationSubnetID              = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/subnet-id"  // both annotation and cloud-config
	// ServiceAnnotationNetworkID             = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/network-id" // both annotation and cloud-config
	// ServiceAnnotationOwnedListeners        = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/owned-listeners"
	// ServiceAnnotationCloudLoadBalancerName = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/cloud-loadbalancer-name" // set via annotation
	// ServiceAnnotationLoadBalancerOwner     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/load-balancer-owner"

	// // Node annotations
	ServiceAnnotationTargetNodeLabels = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/target-node-labels"

	// // LB annotations
	ServiceAnnotationLoadBalancerID   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/load-balancer-id"
	ServiceAnnotationLoadBalancerName = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/load-balancer-name" // only set via the annotation
	ServiceAnnotationPackageID        = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/package-id"         // both annotation and cloud-config
	ServiceAnnotationSecurityGroups   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/security-groups"
	ServiceAnnotationTags             = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/tags"
	ServiceAnnotationScheme           = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/scheme"
	ServiceAnnotationEnableAutoscale  = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-autoscale"

	// // Listener annotations
	ServiceAnnotationIdleTimeoutClient     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/idle-timeout-client"     // both annotation and cloud-config
	ServiceAnnotationIdleTimeoutMember     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/idle-timeout-member"     // both annotation and cloud-config
	ServiceAnnotationIdleTimeoutConnection = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/idle-timeout-connection" // both annotation and cloud-config
	ServiceAnnotationInboundCIDRs          = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/inbound-cidrs"

	// // Pool annotations
	ServiceAnnotationPoolAlgorithm   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/pool-algorithm" // both annotation and cloud-config
	ServiceAnnotationTargetType      = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/target-type"
	ServiceAnnotationProxyProtocol   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-proxy-protocol"
	ServiceAnnotationHealthcheckPort = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-port"
	// ServiceAnnotationEnableStickySession = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-sticky-session"
	// ServiceAnnotationEnableTLSEncryption = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-tls-encryption"

	// // Pool healthcheck annotations
	ServiceAnnotationHealthcheckProtocol        = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-protocol"
	ServiceAnnotationHealthcheckIntervalSeconds = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-interval-seconds"
	ServiceAnnotationHealthcheckTimeoutSeconds  = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-timeout-seconds"
	ServiceAnnotationHealthyThresholdCount      = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthy-threshold-count"
	ServiceAnnotationUnhealthyThresholdCount    = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/unhealthy-threshold-count"

	// // Pool healthcheck annotations for HTTP
	ServiceAnnotationHealthcheckPath           = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-path"
	ServiceAnnotationSuccessCodes              = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/success-codes"
	ServiceAnnotationHealthcheckHttpMethod     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-http-method"
	ServiceAnnotationHealthcheckHttpVersion    = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-http-version"
	ServiceAnnotationHealthcheckHttpDomainName = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-http-domain-name"
)

func PointerOf[T any](t T) *T {
	return &t
}

type ServiceConfig struct {
	LoadBalancerID             string
	LoadBalancerName           string
	LoadBalancerType           loadbalancer.CreateOptsTypeOpt
	PackageID                  string
	Scheme                     loadbalancer.CreateOptsSchemeOpt
	IdleTimeoutClient          int
	IdleTimeoutMember          int
	IdleTimeoutConnection      int
	InboundCIDRs               []string
	HealthcheckProtocol        pool.CreateOptsHealthCheckProtocolOpt
	HealthcheckHttpMethod      pool.CreateOptsHealthCheckMethodOpt
	HealthcheckPath            string
	SuccessCodes               string
	HealthcheckHttpVersion     pool.CreateOptsHealthCheckHttpVersionOpt
	HealthcheckHttpDomainName  string
	PoolAlgorithm              pool.CreateOptsAlgorithmOpt
	HealthyThresholdCount      int
	UnhealthyThresholdCount    int
	HealthcheckTimeoutSeconds  int
	HealthcheckIntervalSeconds int
	HealthcheckPort            int
	Tags                       map[string]string
	TargetNodeLabels           map[string]string
	IsAutoCreateSecurityGroup  bool
	SecurityGroups             []string
	EnableProxyProtocol        []string
	EnableAutoscale            bool
	TargetType                 TargetType
}

func NewServiceConfig(pService *apiv1.Service) *ServiceConfig {
	opt := &ServiceConfig{
		LoadBalancerID:             "",
		LoadBalancerName:           "",
		LoadBalancerType:           loadbalancer.CreateOptsTypeOptLayer4,
		PackageID:                  consts.DEFAULT_L4_PACKAGE_ID,
		Scheme:                     loadbalancer.CreateOptsSchemeOptInternet,
		IdleTimeoutClient:          50,
		IdleTimeoutMember:          50,
		IdleTimeoutConnection:      5,
		InboundCIDRs:               []string{"0.0.0.0/0"},
		HealthcheckProtocol:        pool.CreateOptsHealthCheckProtocolOptTCP,
		HealthcheckHttpMethod:      pool.CreateOptsHealthCheckMethodOptGET,
		HealthcheckPath:            "/",
		SuccessCodes:               "200",
		HealthcheckHttpVersion:     pool.CreateOptsHealthCheckHttpVersionOptHttp1,
		HealthcheckHttpDomainName:  "",
		PoolAlgorithm:              pool.CreateOptsAlgorithmOptRoundRobin,
		HealthyThresholdCount:      3,
		UnhealthyThresholdCount:    3,
		HealthcheckTimeoutSeconds:  5,
		HealthcheckIntervalSeconds: 30,
		HealthcheckPort:            0,
		Tags:                       map[string]string{},
		TargetNodeLabels:           map[string]string{},
		IsAutoCreateSecurityGroup:  false,
		SecurityGroups:             []string{},
		EnableProxyProtocol:        []string{},
		EnableAutoscale:            false,
	}
	if pService == nil {
		return opt
	}
	opt.IsAutoCreateSecurityGroup = true
	if option, ok := pService.Annotations[ServiceAnnotationLoadBalancerName]; ok {
		opt.LoadBalancerName = option
	}
	if option, ok := pService.Annotations[ServiceAnnotationPackageID]; ok {
		opt.PackageID = option
	}
	if option, ok := pService.Annotations[ServiceAnnotationScheme]; ok {
		switch option {
		case "internal":
			opt.Scheme = loadbalancer.CreateOptsSchemeOptInternal
		case "internet-facing":
			opt.Scheme = loadbalancer.CreateOptsSchemeOptInternet
		default:
			klog.Warningf("Invalid annotation \"%s\" value, must be \"internal\" or \"internet-facing\"", ServiceAnnotationScheme)
		}
	}

	if option, ok := pService.Annotations[ServiceAnnotationIdleTimeoutClient]; ok {
		opt.IdleTimeoutClient = utils.ParseIntAnnotation(option, ServiceAnnotationIdleTimeoutClient, opt.IdleTimeoutClient)
	}
	if option, ok := pService.Annotations[ServiceAnnotationIdleTimeoutMember]; ok {
		opt.IdleTimeoutMember = utils.ParseIntAnnotation(option, ServiceAnnotationIdleTimeoutMember, opt.IdleTimeoutMember)
	}
	if option, ok := pService.Annotations[ServiceAnnotationIdleTimeoutConnection]; ok {
		opt.IdleTimeoutConnection = utils.ParseIntAnnotation(option, ServiceAnnotationIdleTimeoutConnection, opt.IdleTimeoutConnection)
	}
	if option, ok := pService.Annotations[ServiceAnnotationInboundCIDRs]; ok {
		opt.InboundCIDRs = utils.ParseStringListAnnotation(option, ServiceAnnotationInboundCIDRs)
	}

	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckProtocol]; ok {
		switch option {
		case string(pool.CreateOptsHealthCheckProtocolOptTCP),
			string(pool.CreateOptsHealthCheckProtocolOptHTTP),
			string(pool.CreateOptsHealthCheckProtocolOptPINGUDP),
			string(pool.CreateOptsHealthCheckProtocolOptHTTPs):
			opt.HealthcheckProtocol = pool.CreateOptsHealthCheckProtocolOpt(option)
		default:
			klog.Warningf("Invalid annotation \"%s\" value, must be one of %s, %s, %s, %s",
				ServiceAnnotationHealthcheckProtocol,
				pool.CreateOptsHealthCheckProtocolOptTCP,
				pool.CreateOptsHealthCheckProtocolOptHTTP,
				pool.CreateOptsHealthCheckProtocolOptHTTPs,
				pool.CreateOptsHealthCheckProtocolOptPINGUDP)
		}
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckHttpMethod]; ok {
		switch option {
		case string(pool.CreateOptsHealthCheckMethodOptGET),
			string(pool.CreateOptsHealthCheckMethodOptPUT),
			string(pool.CreateOptsHealthCheckMethodOptPOST):
			opt.HealthcheckHttpMethod = pool.CreateOptsHealthCheckMethodOpt(option)
		default:
			klog.Warningf("Invalid annotation \"%s\" value, must be one of %s, %s, %s", ServiceAnnotationHealthcheckHttpMethod,
				pool.CreateOptsHealthCheckMethodOptGET,
				pool.CreateOptsHealthCheckMethodOptPUT,
				pool.CreateOptsHealthCheckMethodOptPOST)
		}
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckPath]; ok {
		opt.HealthcheckPath = option
	}
	if option, ok := pService.Annotations[ServiceAnnotationSuccessCodes]; ok {
		opt.SuccessCodes = option
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckHttpVersion]; ok {
		switch option {
		case string(pool.CreateOptsHealthCheckHttpVersionOptHttp1),
			string(pool.CreateOptsHealthCheckHttpVersionOptHttp1Minor1):
			opt.HealthcheckHttpVersion = pool.CreateOptsHealthCheckHttpVersionOpt(option)
		default:
			klog.Warningf("Invalid annotation \"%s\" value, must be one of %s, %s", ServiceAnnotationHealthcheckHttpVersion,
				pool.CreateOptsHealthCheckHttpVersionOptHttp1,
				pool.CreateOptsHealthCheckHttpVersionOptHttp1Minor1)
		}
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckHttpDomainName]; ok {
		opt.HealthcheckHttpDomainName = option
	}
	if option, ok := pService.Annotations[ServiceAnnotationPoolAlgorithm]; ok {
		switch option {
		case string(pool.CreateOptsAlgorithmOptRoundRobin),
			string(pool.CreateOptsAlgorithmOptLeastConn),
			string(pool.CreateOptsAlgorithmOptSourceIP):
			opt.PoolAlgorithm = pool.CreateOptsAlgorithmOpt(option)
		default:
			klog.Warningf("Invalid annotation \"%s\" value, must be one of %s, %s, %s", ServiceAnnotationPoolAlgorithm,
				pool.CreateOptsAlgorithmOptRoundRobin,
				pool.CreateOptsAlgorithmOptLeastConn,
				pool.CreateOptsAlgorithmOptSourceIP)
		}
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthyThresholdCount]; ok {
		opt.HealthyThresholdCount = utils.ParseIntAnnotation(option, ServiceAnnotationHealthyThresholdCount, opt.HealthyThresholdCount)
	}
	if option, ok := pService.Annotations[ServiceAnnotationUnhealthyThresholdCount]; ok {
		opt.UnhealthyThresholdCount = utils.ParseIntAnnotation(option, ServiceAnnotationUnhealthyThresholdCount, opt.UnhealthyThresholdCount)
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckTimeoutSeconds]; ok {
		opt.HealthcheckTimeoutSeconds = utils.ParseIntAnnotation(option, ServiceAnnotationHealthcheckTimeoutSeconds, opt.HealthcheckTimeoutSeconds)
	}
	if option, ok := pService.Annotations[ServiceAnnotationHealthcheckIntervalSeconds]; ok {
		opt.HealthcheckIntervalSeconds = utils.ParseIntAnnotation(option, ServiceAnnotationHealthcheckIntervalSeconds, opt.HealthcheckIntervalSeconds)
	}

	if lbID, ok := pService.Annotations[ServiceAnnotationLoadBalancerID]; ok {
		opt.LoadBalancerID = lbID
	}
	if tags, ok := pService.Annotations[ServiceAnnotationTags]; ok {
		opt.Tags = utils.ParseStringMapAnnotation(tags, ServiceAnnotationTags)
	}
	if tnl, ok := pService.Annotations[ServiceAnnotationTargetNodeLabels]; ok {
		opt.TargetNodeLabels = utils.ParseStringMapAnnotation(tnl, ServiceAnnotationTargetNodeLabels)
	}
	if sgs, ok := pService.Annotations[ServiceAnnotationSecurityGroups]; ok {
		opt.IsAutoCreateSecurityGroup = false
		opt.SecurityGroups = utils.ParseStringListAnnotation(sgs, ServiceAnnotationSecurityGroups)
	}
	if port, ok := pService.Annotations[ServiceAnnotationHealthcheckPort]; ok {
		opt.HealthcheckPort = utils.ParseIntAnnotation(port, ServiceAnnotationHealthcheckPort, opt.HealthcheckPort)
	}
	if proxy, ok := pService.Annotations[ServiceAnnotationProxyProtocol]; ok {
		opt.EnableProxyProtocol = utils.ParseStringListAnnotation(proxy, ServiceAnnotationProxyProtocol)
	}
	if autoscale, ok := pService.Annotations[ServiceAnnotationEnableAutoscale]; ok {
		opt.EnableAutoscale = utils.ParseBoolAnnotation(autoscale, ServiceAnnotationEnableAutoscale, opt.EnableAutoscale)
	}
	if option, ok := pService.Annotations[ServiceAnnotationTargetType]; ok {
		switch option {
		case string(TargetTypeInstance):
			opt.TargetType = TargetTypeInstance
		case string(TargetTypeIP):
			opt.TargetType = TargetTypeIP
		default:
			klog.Warningf("Invalid annotation \"%s\" value, must be \"%s\" or \"%s\"", ServiceAnnotationTargetType, string(TargetTypeInstance), string(TargetTypeIP))
		}
	}
	return opt
}

func (s *ServiceConfig) CreateLoadbalancerOptions() *loadbalancer.CreateOpts {
	opt := &loadbalancer.CreateOpts{
		Name:         s.LoadBalancerName,
		PackageID:    s.PackageID,
		Scheme:       s.Scheme,
		SubnetID:     "",
		Type:         s.LoadBalancerType,
		AutoScalable: s.EnableAutoscale,
	}
	return opt
}

func (s *ServiceConfig) CreateListenerOptions(pPort apiv1.ServicePort) *listener.CreateOpts {
	opt := &listener.CreateOpts{
		ListenerName:                "",
		ListenerProtocol:            utils.ParseListenerProtocol(pPort),
		ListenerProtocolPort:        int(pPort.Port),
		CertificateAuthorities:      nil,
		ClientCertificate:           nil,
		DefaultCertificateAuthority: nil,
		DefaultPoolId:               PointerOf(""),
		TimeoutClient:               s.IdleTimeoutClient,
		TimeoutMember:               s.IdleTimeoutMember,
		TimeoutConnection:           s.IdleTimeoutConnection,
		AllowedCidrs:                utils.StringListToString(s.InboundCIDRs),
	}
	return opt
}

func (s *ServiceConfig) CreatePoolOptions(pPort apiv1.ServicePort) *pool.CreateOpts {
	healthMonitor := pool.HealthMonitor{
		HealthyThreshold:    s.HealthyThresholdCount,
		UnhealthyThreshold:  s.UnhealthyThresholdCount,
		Interval:            s.HealthcheckIntervalSeconds,
		Timeout:             s.HealthcheckTimeoutSeconds,
		HealthCheckProtocol: utils.ParseMonitorProtocol(pPort.Protocol, string(s.HealthcheckProtocol)),
	}
	if s.HealthcheckProtocol == pool.CreateOptsHealthCheckProtocolOptHTTP ||
		s.HealthcheckProtocol == pool.CreateOptsHealthCheckProtocolOptHTTPs {
		healthMonitor = pool.HealthMonitor{
			HealthyThreshold:    s.HealthyThresholdCount,
			UnhealthyThreshold:  s.UnhealthyThresholdCount,
			Interval:            s.HealthcheckIntervalSeconds,
			Timeout:             s.HealthcheckTimeoutSeconds,
			HealthCheckProtocol: utils.ParseMonitorProtocol(pPort.Protocol, string(s.HealthcheckProtocol)),
			HealthCheckMethod:   PointerOf(s.HealthcheckHttpMethod),
			HealthCheckPath:     PointerOf(s.HealthcheckPath),
			SuccessCode:         PointerOf(s.SuccessCodes),
			HttpVersion:         PointerOf(s.HealthcheckHttpVersion),
			DomainName:          PointerOf(s.HealthcheckHttpDomainName),
		}
	}
	opt := &pool.CreateOpts{
		PoolName:      "",
		PoolProtocol:  utils.ParsePoolProtocol(s.MappingProtocol(pPort)),
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: healthMonitor,
		Algorithm:     s.PoolAlgorithm,
		Members:       []*pool.Member{},
	}
	for _, name := range s.EnableProxyProtocol {
		if (name == "*" || name == pPort.Name) && pPort.Protocol == apiv1.ProtocolTCP {
			opt.PoolProtocol = pool.CreateOptsProtocolOptProxy
			break
		}
	}
	return opt
}

func (s *ServiceConfig) MappingProtocol(pPort apiv1.ServicePort) string {
	for _, name := range s.EnableProxyProtocol {
		if (name == "*" || name == pPort.Name) && pPort.Protocol == apiv1.ProtocolTCP {
			return string(pool.CreateOptsProtocolOptProxy)
		}
	}
	return string(pPort.Protocol)
}

func (s *ServiceConfig) GenListenerName(clusterName string, pService *apiv1.Service, resourceType string, pPort apiv1.ServicePort) string {
	hash := utils.GenerateHashName(clusterName, pService.Namespace, pService.Name, resourceType)
	name := fmt.Sprintf("%s_%s_%s_%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		utils.TrimString(clusterName, 10),
		utils.TrimString(pService.Namespace, 9),
		utils.TrimString(pService.Name, 9),
		hash,
		utils.TrimString(string(pPort.Protocol), 3),
		pPort.Port)
	return utils.ValidateName(name)
}

func (s *ServiceConfig) GenPoolName(clusterName string, pService *apiv1.Service, resourceType string, pPort apiv1.ServicePort) string {
	realProtocol := s.MappingProtocol(pPort)

	hash := utils.GenerateHashName(clusterName, pService.Namespace, pService.Name, resourceType)
	name := fmt.Sprintf("%s_%s_%s_%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		utils.TrimString(clusterName, 10),
		utils.TrimString(pService.Namespace, 9),
		utils.TrimString(pService.Name, 9),
		hash,
		utils.TrimString(realProtocol, 3),
		pPort.Port)
	return utils.ValidateName(name)
}
