package vngcloud

import (
	"strings"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/consts"
	"github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/compute/v2/extensions/tag"
	"k8s.io/klog/v2"
)

func GetTags(client *client.ServiceClient, projectID string, resourceID string) ([]*lObjects.ResourceTag, error) {
	klog.V(5).Infoln("[API] GetTags: ", "resourceID: ", resourceID)
	opt := tag.NewGetOpts(projectID, resourceID)
	resp, err := tag.Get(client, opt)
	return resp, err
}

func UpdateTags(client *client.ServiceClient, projectID string, resourceID string, tags map[string]string) error {
	klog.V(5).Infoln("[API] UpdateTags: ", "resourceID: ", resourceID, "tags: ", tags)
	optItems := make([]tag.TagRequestItem, 0)
	for key, value := range tags {
		optItems = append(optItems, tag.TagRequestItem{
			Key:      key,
			Value:    value,
			IsEdited: false,
		})
	}
	opt := tag.NewUpdateOpts(projectID, resourceID, &tag.UpdateOpts{
		ResourceType:   "LOAD-BALANCER",
		ResourceID:     resourceID,
		TagRequestList: optItems,
	})
	_, err := tag.Update(client, opt)
	klog.V(5).Infoln("[API] UpdateTags: ", "err: ", err)
	return err
}

func isValidVKSID(id string) bool {
	return len(id) == consts.VKS_CLUSTER_ID_LENGTH && strings.HasPrefix(id, consts.VKS_CLUSTER_ID_PREFIX)
}

func JoinVKSTag(current, id string) string {
	tags := strings.Split(current, consts.VKS_TAGS_SEPARATOR)
	tagsValid := make(map[string]bool)
	for _, tag := range tags {
		if isValidVKSID(tag) {
			tagsValid[tag] = true
		}
	}
	if isValidVKSID(id) {
		tagsValid[id] = true
	}
	newTags := make([]string, 0)
	for tag := range tagsValid {
		newTags = append(newTags, tag)
	}
	return strings.Join(newTags, consts.VKS_TAGS_SEPARATOR)
}

func RemoveVKSTag(current, id string) string {
	tags := strings.Split(current, consts.VKS_TAGS_SEPARATOR)
	tagsValid := make(map[string]bool)
	for _, tag := range tags {
		if isValidVKSID(tag) {
			tagsValid[tag] = true
		}
	}
	delete(tagsValid, id)
	newTags := make([]string, 0)
	for tag := range tagsValid {
		newTags = append(newTags, tag)
	}
	return strings.Join(newTags, consts.VKS_TAGS_SEPARATOR)
}
