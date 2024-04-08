package vngcloud

import (
	"fmt"
	"strings"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/metrics"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils/errors"
	"github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

func FindPolicyByName(client *client.ServiceClient, projectID string, lbID, listenerID, name string) (*lObjects.Policy, error) {
	policyArr, err := ListPolicyOfListener(client, projectID, lbID, listenerID)
	if err != nil {
		klog.Errorln("error when list policy", err)
		return nil, err
	}
	for _, policy := range policyArr {
		if policy.Name == name {
			return policy, nil
		}
	}
	return nil, errors.ErrNotFound
}

func FindPoolByName(client *client.ServiceClient, projectID string, lbID, name string) (*lObjects.Pool, error) {
	pools, err := ListPoolOfLB(client, projectID, lbID)
	if err != nil {
		return nil, err
	}
	for _, pool := range pools {
		if pool.Name == name {
			ipool, err := GetPool(client, projectID, lbID, pool.UUID)
			if err != nil {
				return nil, err
			}
			return ipool, nil
		}
	}
	return nil, errors.ErrNotFound
}

func FindListenerByName(client *client.ServiceClient, projectID string, lbID, name string) (*lObjects.Listener, error) {
	listeners, err := ListListenerOfLB(client, projectID, lbID)
	if err != nil {
		return nil, err
	}
	for _, listener := range listeners {
		if listener.Name == name {
			return listener, nil
		}
	}
	return nil, errors.ErrNotFound
}

func FindListenerByPort(client *client.ServiceClient, projectID string, lbID string, port int) (*lObjects.Listener, error) {
	listeners, err := ListListenerOfLB(client, projectID, lbID)
	if err != nil {
		return nil, err
	}
	for _, listener := range listeners {
		if listener.ProtocolPort == port {
			// if (port == 443 && listener.Protocol != "HTTPS") || (port == 80 && listener.Protocol != "HTTP") {
			// 	klog.Infof("listener %s has wrong protocol %s or wrong port %d", listener.UUID, listener.Protocol, listener.ProtocolPort)
			// 	return nil, fmt.Errorf("listener %s has wrong protocol %s or wrong port %d", listener.UUID, listener.Protocol, listener.ProtocolPort)
			// } ......................................
			return listener, nil
		}
	}
	return nil, errors.ErrNotFound
}

func WaitForLBActive(client *client.ServiceClient, projectID string, lbID string) (*lObjects.LoadBalancer, error) {
	klog.Infof("Waiting for load balancer %s to be ready", lbID)
	var resultLb *lObjects.LoadBalancer

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: consts.WaitLoadbalancerInitDelay,
		Factor:   consts.WaitLoadbalancerFactor,
		Steps:    consts.WaitLoadbalancerActiveSteps,
	}, func() (done bool, err error) {
		mc := metrics.NewMetricContext("loadbalancer", "get")
		lb, err := GetLB(client, projectID, lbID)
		if mc.ObserveReconcile(err) != nil {
			klog.Errorf("failed to get load balancer %s: %v", lbID, err)
			return false, err
		}

		if strings.ToUpper(lb.Status) == consts.ACTIVE_LOADBALANCER_STATUS {
			klog.Infof("Load balancer %s is ready", lbID)
			resultLb = lb
			return true, nil
		}
		if strings.ToUpper(lb.Status) == consts.ERROR_LOADBALANCER_STATUS {
			klog.Errorf("Load balancer %s is error", lbID)
			resultLb = lb
			return true, fmt.Errorf("load balancer %s is error", lbID)
		}

		klog.Infof("Load balancer %s is not ready yet, wating...", lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		err = fmt.Errorf("timeout waiting for the loadbalancer %s with lb status %s", lbID, resultLb.Status)
	}

	return resultLb, err
}

func ComparePoolOptions(ipool *lObjects.Pool, poolOptions *pool.CreateOpts) *pool.UpdateOpts {
	isNeedUpdate := false
	updateOptions := &pool.UpdateOpts{
		Algorithm:     poolOptions.Algorithm,
		Stickiness:    poolOptions.Stickiness,
		TLSEncryption: poolOptions.TLSEncryption,
		HealthMonitor: poolOptions.HealthMonitor,
	}
	if ipool.LoadBalanceMethod != string(poolOptions.Algorithm) ||
		(poolOptions.Stickiness != nil && ipool.Stickiness != *poolOptions.Stickiness) ||
		(poolOptions.TLSEncryption != nil && ipool.TLSEncryption != *poolOptions.TLSEncryption) {
		isNeedUpdate = true
	}
	if ipool.HealthMonitor.HealthyThreshold != poolOptions.HealthMonitor.HealthyThreshold ||
		ipool.HealthMonitor.UnhealthyThreshold != poolOptions.HealthMonitor.UnhealthyThreshold ||
		ipool.HealthMonitor.Interval != poolOptions.HealthMonitor.Interval ||
		ipool.HealthMonitor.Timeout != poolOptions.HealthMonitor.Timeout {
		isNeedUpdate = true
	}
	if ipool.HealthMonitor.HealthCheckProtocol == "HTTP" && poolOptions.HealthMonitor.HealthCheckProtocol == pool.CreateOptsHealthCheckProtocolOptHTTP {
		// domain may return nil
		if ipool.HealthMonitor.HealthCheckPath == nil || *ipool.HealthMonitor.HealthCheckPath != *poolOptions.HealthMonitor.HealthCheckPath ||
			ipool.HealthMonitor.DomainName == nil || *ipool.HealthMonitor.DomainName != *poolOptions.HealthMonitor.DomainName ||
			ipool.HealthMonitor.HttpVersion == nil || *ipool.HealthMonitor.HttpVersion != string(*poolOptions.HealthMonitor.HttpVersion) ||
			ipool.HealthMonitor.HealthCheckMethod == nil || *ipool.HealthMonitor.HealthCheckMethod != string(*poolOptions.HealthMonitor.HealthCheckMethod) ||
			ipool.HealthMonitor.SuccessCode == nil || *ipool.HealthMonitor.SuccessCode != *poolOptions.HealthMonitor.SuccessCode {
			isNeedUpdate = true
		}
	} else if ipool.HealthMonitor.HealthCheckProtocol == "HTTP" && poolOptions.HealthMonitor.HealthCheckProtocol == pool.CreateOptsHealthCheckProtocolOptTCP {
		updateOptions.HealthMonitor.HealthCheckProtocol = pool.CreateOptsHealthCheckProtocolOptHTTP
		updateOptions.HealthMonitor.HealthCheckPath = ipool.HealthMonitor.HealthCheckPath
		updateOptions.HealthMonitor.DomainName = ipool.HealthMonitor.DomainName
		*updateOptions.HealthMonitor.HttpVersion = pool.CreateOptsHealthCheckHttpVersionOpt(*ipool.HealthMonitor.HttpVersion)
		*updateOptions.HealthMonitor.HealthCheckMethod = pool.CreateOptsHealthCheckMethodOpt(*ipool.HealthMonitor.HealthCheckMethod)
	} else if ipool.HealthMonitor.HealthCheckProtocol == "TCP" && poolOptions.HealthMonitor.HealthCheckProtocol == pool.CreateOptsHealthCheckProtocolOptHTTP {
		updateOptions.HealthMonitor.HealthCheckProtocol = pool.CreateOptsHealthCheckProtocolOptTCP
		updateOptions.HealthMonitor.HealthCheckPath = nil
		updateOptions.HealthMonitor.DomainName = nil
		updateOptions.HealthMonitor.HttpVersion = nil
		updateOptions.HealthMonitor.HealthCheckMethod = nil
	}

	if !isNeedUpdate {
		return nil
	}
	return updateOptions
}

func CheckIfPoolMemberExist(mems []*pool.Member, mem *pool.Member) bool {
	for _, r := range mems {
		if r.IpAddress == mem.IpAddress &&
			r.Port == mem.Port &&
			r.MonitorPort == mem.MonitorPort &&
			r.Backup == mem.Backup &&
			// r.Name == mem.Name &&
			r.Weight == mem.Weight {
			return true
		}
	}
	return false
}

func ConvertObjectToPoolMember(obj *lObjects.Member) *pool.Member {
	return &pool.Member{
		IpAddress:   obj.Address,
		Port:        obj.ProtocolPort,
		MonitorPort: obj.MonitorPort,
		Backup:      obj.Backup,
		Weight:      obj.Weight,
		Name:        obj.Name,
	}
}

func ConvertObjectToPoolMemberArray(obj []*lObjects.Member) []*pool.Member {
	ret := make([]*pool.Member, len(obj))
	for i, m := range obj {
		ret[i] = ConvertObjectToPoolMember(m)
	}
	return ret
}

func ComparePoolMembers(p1, p2 []*pool.Member) bool {
	if len(p1) != len(p2) {
		return false
	}
	for _, m := range p2 {
		if !CheckIfPoolMemberExist(p1, m) {
			klog.Infof("member in pool not exist: %v", m)
			return false
		}
	}
	return true
}

func CompareListenerOptions(ilis *lObjects.Listener, lisOptions *listener.CreateOpts) *listener.UpdateOpts {
	isNeedUpdate := false
	updateOptions := &listener.UpdateOpts{
		AllowedCidrs:                lisOptions.AllowedCidrs,
		TimeoutClient:               lisOptions.TimeoutClient,
		TimeoutMember:               lisOptions.TimeoutMember,
		TimeoutConnection:           lisOptions.TimeoutConnection,
		DefaultPoolId:               lisOptions.DefaultPoolId,
		DefaultCertificateAuthority: lisOptions.DefaultCertificateAuthority,
		// Headers:                     lisOptions.Headers,
		// ClientCertificate:           lisOptions.ClientCertificateAuthentication,
		// ......................................... update later
	}
	if ilis.AllowedCidrs != lisOptions.AllowedCidrs ||
		ilis.TimeoutClient != lisOptions.TimeoutClient ||
		ilis.TimeoutMember != lisOptions.TimeoutMember ||
		ilis.TimeoutConnection != lisOptions.TimeoutConnection {
		isNeedUpdate = true
	}

	if ilis.DefaultPoolId != lisOptions.DefaultPoolId {
		klog.Infof("listener need update default pool id: %s", lisOptions.DefaultPoolId)
		isNeedUpdate = true
	}
	if lisOptions.DefaultCertificateAuthority != nil && (ilis.DefaultCertificateAuthority == nil || *(ilis.DefaultCertificateAuthority) != *(lisOptions.DefaultCertificateAuthority)) {
		klog.Infof("listener need update default certificate authority: %s", *lisOptions.DefaultCertificateAuthority)
		isNeedUpdate = true
	}
	// update cert SNI here .......................................................
	if !isNeedUpdate {
		return nil
	}
	return updateOptions
}
