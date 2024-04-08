package metadata

import (
	"errors"
	"sync"
)

var (
	metadataIns     IMetadata
	metadataInsOnce sync.Once
)

var (
	metadataCache  *Metadata
	ErrBadMetadata = errors.New("invalid OpenStack metadata, got empty uuid")
)

const (
	ConfigDriveID           = "configDrive"
	MetadataID              = "metadataService"
	configDriveLabel        = "config-2"
	defaultMetadataVersion  = "latest"
	configDrivePathTemplate = "openstack/%s/meta_data.json"
	metadataURLTemplate     = "http://169.254.169.254/openstack/%s/meta_data.json"
)
