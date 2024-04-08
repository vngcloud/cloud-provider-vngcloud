package metadata

type metadataService struct {
	searchOrder string
}

func (s *metadataService) GetInstanceID() (string, error) {
	md, err := Get(s.searchOrder)
	if err != nil {
		return "", err
	}
	return md.UUID, nil
}

func (s *metadataService) GetAvailabilityZone() (string, error) {
	md, err := Get(s.searchOrder)
	if err != nil {
		return "", err
	}
	return md.AvailabilityZone, nil
}

func (s *metadataService) GetProjectID() (string, error) {
	md, err := Get(s.searchOrder)
	if err != nil {
		return "", err
	}
	return md.ProjectID, nil
}
