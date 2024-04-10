package utils

import (
	"fmt"

	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/policy"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/extensions/secgroup_rule"
	"k8s.io/klog/v2"
)

type PoolExpander struct {
	UUID string
	pool.CreateOpts
}

type PolicyExpander struct {
	IsInUse         bool
	IsHttpsListener bool
	ListenerID      string
	// shouldForwardToPoolName string

	UUID             string
	Name             string
	RedirectPoolID   string
	RedirectPoolName string
	Action           policy.PolicyOptsActionOpt
	L7Rules          []policy.Rule
	// *lObjects.Policy
}

type ListenerExpander struct {
	UUID            string
	DefaultPoolName string // use for L4 only
	listener.CreateOpts
}
type CertificateExpander struct {
	UUID       string
	Name       string
	Version    string
	SecretName string
}

type SecGroupRuleExpander struct {
	UUID string
	secgroup_rule.CreateOpts
}

type IngressInspect struct {
	DefaultPool *PoolExpander
	Name        string
	Namespace   string
	LbOptions   *loadbalancer.CreateOpts // create options for lb
	AllowCIDR   string

	PolicyExpander       []*PolicyExpander
	PoolExpander         []*PoolExpander
	ListenerExpander     []*ListenerExpander
	CertificateExpander  []*CertificateExpander
	SecurityGroups       []string
	InstanceIDs          []string
	SecGroupRuleExpander []*SecGroupRuleExpander
}

func (ing *IngressInspect) Print() {
	fmt.Println("DEFAULT POOL: name:", ing.DefaultPool.PoolName, "id:", ing.DefaultPool.UUID, "members:", ing.DefaultPool.Members)
	for _, l := range ing.ListenerExpander {
		fmt.Println("LISTENER: id:", l.UUID)
	}
	for _, p := range ing.PolicyExpander {
		fmt.Println("---- POLICY: id:", p.UUID, "name:", p.Name, "redirectPoolName:", p.RedirectPoolName, "redirectPoolId:", p.RedirectPoolID, "action:", p.Action, "l7Rules:", p.L7Rules)
	}
	for _, p := range ing.PoolExpander {
		fmt.Println("++++ POOL: name:", p.PoolName, "uuid:", p.UUID, "members:", p.Members)
	}
}

func MapIDExpander(old, cur *IngressInspect) {
	// map policy
	mapPolicyIndex := make(map[string]int)
	for curIndex, curPol := range cur.PolicyExpander {
		mapPolicyIndex[curPol.Name] = curIndex
	}
	for _, oldPol := range old.PolicyExpander {
		if curIndex, ok := mapPolicyIndex[oldPol.Name]; ok {
			oldPol.UUID = cur.PolicyExpander[curIndex].UUID
			oldPol.ListenerID = cur.PolicyExpander[curIndex].ListenerID
		} else {
			klog.Errorf("policy not found when map ingress: %v", oldPol)
		}
	}

	// map pool
	mapPoolIndex := make(map[string]int)
	for curIndex, curPol := range cur.PoolExpander {
		mapPoolIndex[curPol.PoolName] = curIndex
	}
	for _, oldPol := range old.PoolExpander {
		if curIndex, ok := mapPoolIndex[oldPol.PoolName]; ok {
			oldPol.UUID = cur.PoolExpander[curIndex].UUID
		} else {
			klog.Errorf("pool not found when map ingress: %v", oldPol)
		}
	}

	// map listener
	mapListenerIndex := make(map[string]int)
	for curIndex, curPol := range cur.ListenerExpander {
		mapListenerIndex[curPol.CreateOpts.ListenerName] = curIndex
	}
	for _, oldPol := range old.ListenerExpander {
		if curIndex, ok := mapListenerIndex[oldPol.CreateOpts.ListenerName]; ok {
			oldPol.UUID = cur.ListenerExpander[curIndex].UUID
		} else {
			klog.Errorf("listener not found when map ingress: %v", oldPol)
		}
	}
}

func (ing *IngressInspect) AddSecgroupRule(port int, protocol secgroup_rule.CreateOptsProtocolOpt) {
	isExist := false
	for _, rule := range ing.SecGroupRuleExpander {
		if rule.PortRangeMax == port &&
			rule.PortRangeMin == port &&
			rule.Protocol == protocol {
			isExist = true
			break
		}
	}
	if !isExist {
		ing.SecGroupRuleExpander = append(ing.SecGroupRuleExpander, &SecGroupRuleExpander{
			CreateOpts: secgroup_rule.CreateOpts{
				Description:     "",
				Direction:       secgroup_rule.CreateOptsDirectionOptIngress,
				EtherType:       secgroup_rule.CreateOptsEtherTypeOptIPv4,
				PortRangeMax:    port,
				PortRangeMin:    port,
				Protocol:        protocol,
				RemoteIPPrefix:  "",
				SecurityGroupID: "",
			},
		})
	}
}
