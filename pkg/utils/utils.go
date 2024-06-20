package utils

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	lStr "strings"
	"time"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	lConsts "github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	lListenerV2 "github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/listener"
	lLoadBalancerV2 "github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/loadbalancer"
	lPoolV2 "github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/pool"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

type MyDuration struct {
	time.Duration
}

// PatchService makes patch request to the Service object.
func PatchService(ctx context.Context, client clientset.Interface, cur, mod *apiv1.Service) error {
	curJSON, err := json.Marshal(cur)
	if err != nil {
		return fmt.Errorf("failed to serialize current service object: %s", err)
	}

	modJSON, err := json.Marshal(mod)
	if err != nil {
		return fmt.Errorf("failed to serialize modified service object: %s", err)
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(curJSON, modJSON, apiv1.Service{})
	if err != nil {
		return fmt.Errorf("failed to create 2-way merge patch: %s", err)
	}

	if len(patch) == 0 || string(patch) == "{}" {
		return nil
	}

	_, err = client.CoreV1().Services(cur.Namespace).Patch(ctx, cur.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch service object %s/%s: %s", cur.Namespace, cur.Name, err)
	}

	return nil
}

func IsPoolProtocolValid(pPool *lObjects.Pool, pPort apiv1.ServicePort, pPoolName string) bool {
	if pPool != nil &&
		!lStr.EqualFold(lStr.TrimSpace(pPool.Protocol), lStr.TrimSpace(string(pPort.Protocol))) &&
		pPoolName == pPool.Name {
		return false
	}

	return true
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ListWorkerNodes(pNodes []*apiv1.Node, pOnlyReadyNode bool) []*apiv1.Node {
	var workerNodes []*apiv1.Node

	for _, node := range pNodes {
		// Ignore master nodes
		if _, ok := node.GetObjectMeta().GetLabels()[lConsts.DEFAULT_K8S_MASTER_LABEL]; ok {
			continue
		}

		// If the status of node is not considered, add it to the list
		if !pOnlyReadyNode {
			workerNodes = append(workerNodes, node)
			continue
		}

		// If this worker does not have any condition, ignore it
		if len(node.Status.Conditions) < 1 {
			continue
		}

		// Only consider ready nodes
		for _, condition := range node.Status.Conditions {
			if condition.Type == apiv1.NodeReady && condition.Status != apiv1.ConditionTrue {
				continue
			}
		}

		// This is a truly well worker, add it to the list
		workerNodes = append(workerNodes, node)
	}

	return workerNodes
}

func ParsePoolAlgorithm(pOpt string) lPoolV2.CreateOptsAlgorithmOpt {
	opt := lStr.ReplaceAll(lStr.TrimSpace(lStr.ToUpper(pOpt)), "-", "_")
	switch opt {
	case string(lPoolV2.CreateOptsAlgorithmOptSourceIP):
		return lPoolV2.CreateOptsAlgorithmOptSourceIP
	case string(lPoolV2.CreateOptsAlgorithmOptLeastConn):
		return lPoolV2.CreateOptsAlgorithmOptLeastConn
	}
	return lPoolV2.CreateOptsAlgorithmOptRoundRobin
}

func ParsePoolProtocol(pPoolProtocol string) lPoolV2.CreateOptsProtocolOpt {
	opt := lStr.TrimSpace(lStr.ToUpper(string(pPoolProtocol)))
	switch opt {
	case string(lPoolV2.CreateOptsProtocolOptProxy):
		return lPoolV2.CreateOptsProtocolOptProxy
	case string(lPoolV2.CreateOptsProtocolOptHTTP):
		return lPoolV2.CreateOptsProtocolOptHTTP
	case string(lPoolV2.CreateOptsProtocolOptUDP):
		return lPoolV2.CreateOptsProtocolOptUDP
	}
	return lPoolV2.CreateOptsProtocolOptTCP
}

func ParseMonitorProtocol(
	pPoolProtocol apiv1.Protocol, pMonitorProtocol string) lPoolV2.CreateOptsHealthCheckProtocolOpt {

	switch lStr.TrimSpace(lStr.ToUpper(string(pPoolProtocol))) {
	case string(lPoolV2.CreateOptsProtocolOptUDP):
		return lPoolV2.CreateOptsHealthCheckProtocolOptPINGUDP
	}

	switch lStr.TrimSpace(lStr.ToUpper(pMonitorProtocol)) {
	case string(lPoolV2.CreateOptsHealthCheckProtocolOptHTTP):
		return lPoolV2.CreateOptsHealthCheckProtocolOptHTTP
	case string(lPoolV2.CreateOptsHealthCheckProtocolOptHTTPs):
		return lPoolV2.CreateOptsHealthCheckProtocolOptHTTPs
	case string(lPoolV2.CreateOptsHealthCheckProtocolOptPINGUDP):
		return lPoolV2.CreateOptsHealthCheckProtocolOptPINGUDP
	}

	return lPoolV2.CreateOptsHealthCheckProtocolOptTCP
}

func ParseMonitorHealthCheckMethod(pMethod string) *lPoolV2.CreateOptsHealthCheckMethodOpt {
	opt := lStr.TrimSpace(lStr.ToUpper(pMethod))
	switch opt {
	case string(lPoolV2.CreateOptsHealthCheckMethodOptPUT):
		tmp := lPoolV2.CreateOptsHealthCheckMethodOptPUT
		return &tmp
	case string(lPoolV2.CreateOptsHealthCheckMethodOptPOST):
		tmp := lPoolV2.CreateOptsHealthCheckMethodOptPOST
		return &tmp
	}

	tmp := lPoolV2.CreateOptsHealthCheckMethodOptGET
	return &tmp
}

func ParseMonitorHttpVersion(pVersion string) *lPoolV2.CreateOptsHealthCheckHttpVersionOpt {
	opt := lStr.TrimSpace(lStr.ToUpper(pVersion))
	switch opt {
	case string(lPoolV2.CreateOptsHealthCheckHttpVersionOptHttp1Minor1):
		tmp := lPoolV2.CreateOptsHealthCheckHttpVersionOptHttp1Minor1
		return &tmp
	}

	tmp := lPoolV2.CreateOptsHealthCheckHttpVersionOptHttp1
	return &tmp
}

func ParseLoadBalancerScheme(pInternal bool) lLoadBalancerV2.CreateOptsSchemeOpt {
	if pInternal {
		return lLoadBalancerV2.CreateOptsSchemeOptInternal
	}
	return lLoadBalancerV2.CreateOptsSchemeOptInternet
}

func ParseListenerProtocol(pPort apiv1.ServicePort) lListenerV2.CreateOptsListenerProtocolOpt {
	opt := lStr.TrimSpace(lStr.ToUpper(string(pPort.Protocol)))
	switch opt {
	case string(lListenerV2.CreateOptsListenerProtocolOptUDP):
		return lListenerV2.CreateOptsListenerProtocolOptUDP
	}

	return lListenerV2.CreateOptsListenerProtocolOptTCP
}

func GetStringFromServiceAnnotation(pService *apiv1.Service, annotationKey string, defaultSetting string) string {
	klog.V(4).Infof("getStringFromServiceAnnotation(%s/%s, %v, %v)", pService.Namespace, pService.Name, annotationKey, defaultSetting)
	if annotationValue, ok := pService.Annotations[annotationKey]; ok {
		//if there is an annotation for this setting, set the "setting" var to it
		// annotationValue can be empty, it is working as designed
		// it makes possible for instance provisioning loadbalancer without floatingip
		klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
		return annotationValue
	}

	//if there is no annotation, set "settings" var to the value from cloud config
	if defaultSetting != "" {
		klog.V(4).Infof("Could not find a Service Annotation; falling back on cloud-config setting: %v = %v", annotationKey, defaultSetting)
	}

	return defaultSetting
}

func GetIntFromServiceAnnotation(service *apiv1.Service, annotationKey string, defaultSetting int) int {
	klog.V(4).Infof("getIntFromServiceAnnotation(%s/%s, %v, %v)", service.Namespace, service.Name, annotationKey, defaultSetting)
	if annotationValue, ok := service.Annotations[annotationKey]; ok {
		returnValue, err := strconv.Atoi(annotationValue)
		if err != nil {
			klog.Warningf("Could not parse int value from %q, failing back to default %s = %v, %v", annotationValue, annotationKey, defaultSetting, err)
			return defaultSetting
		}

		klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
		return returnValue
	}
	klog.V(4).Infof("Could not find a Service Annotation; falling back to default setting: %v = %v", annotationKey, defaultSetting)
	return defaultSetting
}

func TrimString(str string, length int) string {
	return str[:MinInt(len(str), length)]
}

// HashString hash a string to a string have 10 char
func HashString(str string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
}

func ParseIntAnnotation(s, annotation string, defaultValue int) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		klog.Warningf("Invalid annotation \"%s\" value, use default value = %d", annotation, defaultValue)
		return defaultValue
	}
	return i
}

func ParseBoolAnnotation(s, annotation string, defaultValue bool) bool {
	b, err := strconv.ParseBool(s)
	if err != nil {
		klog.Warningf("Invalid annotation \"%s\" value, use default value = %t", annotation, defaultValue)
		return defaultValue
	}
	return b
}

func ParseStringListAnnotation(s, annotation string) []string {
	if s == "" {
		return []string{}
	}
	ss := strings.Split(s, ",")
	validStr := []string{}
	for _, v := range ss {
		str := strings.TrimSpace(v)
		if str != "" {
			validStr = append(validStr, str)
		}
	}
	return validStr
}

func StringListToString(s []string) string {
	return strings.Join(s, ",")
}

func ParseStringMapAnnotation(s, annotation string) map[string]string {
	if s == "" {
		return map[string]string{}
	}
	ss := strings.Split(s, ",")
	validStr := map[string]string{}
	for _, v := range ss {
		str := strings.TrimSpace(v)
		if str != "" {
			kv := strings.Split(str, "=")
			if len(kv) == 2 && kv[0] != "" && kv[1] != "" {
				validStr[kv[0]] = kv[1]
			}
		}
	}
	return validStr
}

func ListNodeWithPredicate(nodeLister corelisters.NodeLister, nodeLabels map[string]string) ([]*apiv1.Node, error) {
	labelSelector := labels.SelectorFromSet(nodeLabels)
	nodes, err := nodeLister.List(labelSelector)
	if err != nil {
		return nil, err
	}

	var filtered []*apiv1.Node
	for i := range nodes {
		if getNodeConditionPredicate(nodes[i]) {
			filtered = append(filtered, nodes[i])
		}
	}

	return filtered, nil
}

func getNodeConditionPredicate(node *apiv1.Node) bool {
	// We add the master to the node list, but its unschedulable.  So we use this to filter
	// the master.
	if node.Spec.Unschedulable {
		return false
	}

	// Recognize nodes labeled as not suitable for LB, and filter them also, as we were doing previously.
	if _, hasExcludeLBRoleLabel := node.Labels[consts.LabelNodeExcludeLB]; hasExcludeLBRoleLabel {
		return false
	}

	// Deprecated in favor of LabelNodeExcludeLB, kept for consistency and will be removed later
	if _, hasNodeRoleMasterLabel := node.Labels[consts.LabelNodeExcludeLB]; hasNodeRoleMasterLabel {
		return false
	}

	// If we have no info, don't accept
	if len(node.Status.Conditions) == 0 {
		return false
	}
	for _, cond := range node.Status.Conditions {
		// We consider the node for load balancing only when its NodeReady condition status
		// is ConditionTrue
		if cond.Type == apiv1.NodeReady && cond.Status != apiv1.ConditionTrue {
			klog.Info("ignoring node:", "name", node.Name, "status", cond.Status)
			// log.WithFields(log.Fields{"name": node.Name, "status": cond.Status}).Info("ignoring node")
			return false
		}
	}
	return true
}
func ListServiceWithPredicate(serviceLister corelisters.ServiceLister) ([]*apiv1.Service, error) {
	services, err := serviceLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var filtered []*apiv1.Service
	for i := range services {
		if getServiceConditionPredicate(services[i]) {
			filtered = append(filtered, services[i])
		}
	}

	return filtered, nil
}
func getServiceConditionPredicate(service *apiv1.Service) bool {
	// We only consider services with type LoadBalancer
	return service.Spec.Type == apiv1.ServiceTypeLoadBalancer
}

// providerID
const (
	// Define the regular expression pattern
	patternPrefix = `vngcloud:\/\/`
	rawPrefix     = `vngcloud://`
	pattern       = "^" + patternPrefix + "ins-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

var (
	vngCloudProviderIDRegex = regexp.MustCompile(pattern)
)

func GetListProviderID(pnodes []*apiv1.Node) []string {
	var providerIDs []string
	for _, node := range pnodes {
		if node != nil && (matchCloudProviderPattern(node.Spec.ProviderID)) {
			providerIDs = append(providerIDs, getProviderID(node))
		}
	}

	return providerIDs
}

func matchCloudProviderPattern(pproviderID string) bool {
	return vngCloudProviderIDRegex.MatchString(pproviderID)
}

func getProviderID(pnode *apiv1.Node) string {
	return pnode.Spec.ProviderID[len(rawPrefix):len(pnode.Spec.ProviderID)]
}

func MergeStringArray(current, remove, add []string) ([]string, bool) {
	klog.V(5).Infof("current: %v", current)
	klog.V(5).Infof("remove:  %v", remove)
	klog.V(5).Infof("add:     %v", add)
	mapCurrent := make(map[string]bool)
	for _, c := range current {
		mapCurrent[c] = true
	}
	for _, r := range remove {
		delete(mapCurrent, r)
	}
	for _, a := range add {
		mapCurrent[a] = true
	}
	ret := make([]string, 0)
	for k := range mapCurrent {
		ret = append(ret, k)
	}
	if len(ret) != len(current) {
		return ret, true
	}
	for _, c := range current {
		if !mapCurrent[c] {
			return ret, true
		}
	}
	return ret, false
}
