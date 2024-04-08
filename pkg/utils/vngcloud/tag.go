package vngcloud

import (
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
