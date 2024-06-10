package utils

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"k8s.io/klog/v2"
)

func GenerateHashName(clusterID, namespace, resourceName, resourceType string) string {
	fullName := fmt.Sprintf("%s_%s_%s_%s", clusterID, namespace, resourceName, resourceType)
	hash := HashString(fullName)
	return TrimString(hash, consts.DEFAULT_HASH_NAME_LENGTH)
}

func GenerateLBName(clusterID, namespace, resourceName, resourceType string) string {
	hash := GenerateHashName(clusterID, namespace, resourceName, resourceType)
	name := fmt.Sprintf("%s_%s_%s_%s_%s",
		consts.DEFAULT_LB_PREFIX_NAME,
		TrimString(clusterID, 10),
		TrimString(namespace, 10),
		TrimString(resourceName, 10),
		hash)
	return ValidateName(name)
}

func GeneratePolicyName(clusterID, namespace, resourceName, resourceType string, mode bool, ruleIndex, pathIndex int) string {
	prefix := GenerateHashName(clusterID, namespace, resourceName, resourceType)
	name := fmt.Sprintf("%s_%s_%t_r%d_p%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		prefix, mode, ruleIndex, pathIndex)
	return ValidateName(name)
}

func GeneratePoolName(clusterID, namespace, resourceName, resourceType, serviceName string, port int) string {
	prefix := GenerateHashName(clusterID, namespace, resourceName, resourceType)
	name := fmt.Sprintf("%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		prefix,
		TrimString(strings.ReplaceAll(serviceName, "/", "-"), 35),
		port)
	return ValidateName(name)
}

func ValidateName(newName string) string {
	for _, char := range newName {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '-' && char != '.' {
			newName = strings.ReplaceAll(newName, string(char), "-")
		}
	}
	if len(newName) > consts.DEFAULT_PORTAL_NAME_LENGTH {
		klog.Warningf("The name %s is too long, it will be truncated", newName)
	}
	return TrimString(newName, consts.DEFAULT_PORTAL_NAME_LENGTH)
}
