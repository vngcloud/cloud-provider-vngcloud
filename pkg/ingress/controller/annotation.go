package controller

import (
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	nwv1 "k8s.io/api/networking/v1"
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

	// Node annotations
	ServiceAnnotationTargetNodeLabels = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/target-node-labels"

	// LB annotations
	ServiceAnnotationLoadBalancerID   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/load-balancer-id"
	ServiceAnnotationLoadBalancerName = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/load-balancer-name" // only set via the annotation
	ServiceAnnotationPackageID        = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/package-id"         // both annotation and cloud-config
	ServiceAnnotationSecurityGroups   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/security-groups"
	ServiceAnnotationTags             = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/tags"
	ServiceAnnotationScheme           = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/scheme"
	ServiceAnnotationCertificateIDs   = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/certificate-ids"
	ServiceAnnotationEnableAutoscale  = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-autoscale"

	// Listener annotations
	ServiceAnnotationIdleTimeoutClient     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/idle-timeout-client"     // both annotation and cloud-config
	ServiceAnnotationIdleTimeoutMember     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/idle-timeout-member"     // both annotation and cloud-config
	ServiceAnnotationIdleTimeoutConnection = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/idle-timeout-connection" // both annotation and cloud-config
	ServiceAnnotationInboundCIDRs          = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/inbound-cidrs"

	// Pool annotations
	ServiceAnnotationPoolAlgorithm       = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/pool-algorithm" // both annotation and cloud-config
	ServiceAnnotationTargetType          = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/target-type"
	ServiceAnnotationEnableStickySession = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-sticky-session"
	ServiceAnnotationEnableTLSEncryption = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/enable-tls-encryption"
	ServiceAnnotationHealthcheckPort     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-port"

	// Pool healthcheck annotations
	ServiceAnnotationHealthcheckProtocol        = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-protocol"
	ServiceAnnotationHealthcheckIntervalSeconds = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-interval-seconds"
	ServiceAnnotationHealthcheckTimeoutSeconds  = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-timeout-seconds"
	ServiceAnnotationHealthyThresholdCount      = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthy-threshold-count"
	ServiceAnnotationUnhealthyThresholdCount    = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/unhealthy-threshold-count"

	// Pool healthcheck annotations for HTTP
	ServiceAnnotationHealthcheckPath           = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-path"
	ServiceAnnotationSuccessCodes              = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/success-codes"
	ServiceAnnotationHealthcheckHttpMethod     = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-http-method"
	ServiceAnnotationHealthcheckHttpVersion    = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-http-version"
	ServiceAnnotationHealthcheckHttpDomainName = DEFAULT_K8S_SERVICE_ANNOTATION_PREFIX + "/healthcheck-http-domain-name"
)

func PointerOf[T any](t T) *T {
	return &t
}

type IngressConfig struct {
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
	EnableStickySession        bool
	EnableTLSEncryption        bool
	CertificateIDs             []string
	EnableAutoscale            bool
	TargetType                 TargetType
}

func NewIngressConfig(pService *nwv1.Ingress) *IngressConfig {
	opt := &IngressConfig{
		LoadBalancerID:             "",
		LoadBalancerName:           "",
		LoadBalancerType:           loadbalancer.CreateOptsTypeOptLayer7,
		PackageID:                  consts.DEFAULT_L7_PACKAGE_ID,
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
		EnableStickySession:        false,
		EnableTLSEncryption:        false,
		CertificateIDs:             []string{},
		EnableAutoscale:            false,
		TargetType:                 TargetTypeInstance,
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
	if option, ok := pService.Annotations[ServiceAnnotationEnableStickySession]; ok {
		opt.EnableStickySession = utils.ParseBoolAnnotation(option, ServiceAnnotationEnableStickySession, opt.EnableStickySession)
	}
	if option, ok := pService.Annotations[ServiceAnnotationEnableTLSEncryption]; ok {
		opt.EnableTLSEncryption = utils.ParseBoolAnnotation(option, ServiceAnnotationEnableTLSEncryption, opt.EnableTLSEncryption)
	}
	if option, ok := pService.Annotations[ServiceAnnotationCertificateIDs]; ok {
		arr := utils.ParseStringListAnnotation(option, ServiceAnnotationCertificateIDs)
		// remove duplicate certificate IDs
		mapCertIDs := make(map[string]bool)
		result := []string{}
		for _, certID := range arr {
			if !mapCertIDs[certID] {
				result = append(result, certID)
				mapCertIDs[certID] = true
			}
		}
		opt.CertificateIDs = result
	}
	if option, ok := pService.Annotations[ServiceAnnotationEnableAutoscale]; ok {
		opt.EnableAutoscale = utils.ParseBoolAnnotation(option, ServiceAnnotationEnableAutoscale, opt.EnableAutoscale)
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

func (s *IngressConfig) CreateLoadbalancerOptions() *loadbalancer.CreateOpts {
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

func (s *IngressConfig) CreateListenerOptions(isHTTPS bool) *listener.CreateOpts {
	opt := &listener.CreateOpts{
		ListenerName:                consts.DEFAULT_HTTP_LISTENER_NAME,
		ListenerProtocol:            listener.CreateOptsListenerProtocolOptHTTP,
		ListenerProtocolPort:        80,
		CertificateAuthorities:      nil,
		ClientCertificate:           nil,
		DefaultCertificateAuthority: nil,
		DefaultPoolId:               PointerOf(""),
		TimeoutClient:               s.IdleTimeoutClient,
		TimeoutMember:               s.IdleTimeoutMember,
		TimeoutConnection:           s.IdleTimeoutConnection,
		AllowedCidrs:                utils.StringListToString(s.InboundCIDRs),
	}
	if isHTTPS {
		opt.ListenerName = consts.DEFAULT_HTTPS_LISTENER_NAME
		opt.ListenerProtocol = listener.CreateOptsListenerProtocolOptHTTPS
		opt.ListenerProtocolPort = 443
	}
	return opt
}

func (s *IngressConfig) CreatePoolOptions() *pool.CreateOpts {
	healthMonitor := pool.HealthMonitor{
		HealthyThreshold:    s.HealthyThresholdCount,
		UnhealthyThreshold:  s.UnhealthyThresholdCount,
		Interval:            s.HealthcheckIntervalSeconds,
		Timeout:             s.HealthcheckTimeoutSeconds,
		HealthCheckProtocol: s.HealthcheckProtocol,
	}
	if s.HealthcheckProtocol == pool.CreateOptsHealthCheckProtocolOptHTTP {
		healthMonitor = pool.HealthMonitor{
			HealthyThreshold:    s.HealthyThresholdCount,
			UnhealthyThreshold:  s.UnhealthyThresholdCount,
			Interval:            s.HealthcheckIntervalSeconds,
			Timeout:             s.HealthcheckTimeoutSeconds,
			HealthCheckProtocol: s.HealthcheckProtocol,
			HealthCheckMethod:   PointerOf(s.HealthcheckHttpMethod),
			HealthCheckPath:     PointerOf(s.HealthcheckPath),
			SuccessCode:         PointerOf(s.SuccessCodes),
			HttpVersion:         PointerOf(s.HealthcheckHttpVersion),
			DomainName:          PointerOf(s.HealthcheckHttpDomainName),
		}
	}
	opt := &pool.CreateOpts{
		PoolName:      "",
		PoolProtocol:  pool.CreateOptsProtocolOptHTTP,
		Stickiness:    PointerOf(s.EnableStickySession),
		TLSEncryption: PointerOf(s.EnableStickySession),
		HealthMonitor: healthMonitor,
		Algorithm:     s.PoolAlgorithm,
		Members:       []*pool.Member{},
	}
	return opt
}
