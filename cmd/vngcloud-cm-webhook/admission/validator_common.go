package admission

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
)

// commonValidator is a container for validating the name
type commonValidator struct {
	Logger logrus.FieldLogger
}

// commonValidator implements the itemValidator interface
var _ itemValidator = (*commonValidator)(nil)

// Name returns the name of commonValidator
func (n commonValidator) Name() string {
	return "name_validator"
}

// Validate inspects the name of a given item and returns validation.
// The returned validation is only valid if the item name does not contain some
// bad string.
func (n commonValidator) Validate(a *admissionv1.AdmissionRequest) (validation, error) {
	valid := validation{Valid: true, Reason: "valid"}
	if a.Kind.Kind != "Service" {
		return validation{
			Valid:  false,
			Reason: fmt.Sprintf("expected kind Service, got %q", a.Kind.Kind),
		}, nil
	}
	service, err := toService(a.Object.Raw)
	if err != nil {
		return validation{
			Valid:  false,
			Reason: fmt.Sprintf("could not parse service in admission review request: %v", err),
		}, err
	}
	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return valid, nil
	}

	// Prevent changing the load balancer name
	oldService := service
	if a.Operation == admissionv1.Update {
		oldService, err = toService(a.OldObject.Raw)
		if err != nil {
			return validation{
				Valid:  false,
				Reason: fmt.Sprintf("could not parse old service in admission review request: %v", err),
			}, err
		}
		if oldService.Spec.Type != corev1.ServiceTypeLoadBalancer {
			return valid, nil
		}

		staticAnnotationKey := "vks.vngcloud.vn/load-balancer-name"
		if oldService.Annotations[staticAnnotationKey] != service.Annotations[staticAnnotationKey] && oldService.Annotations[staticAnnotationKey] != "" {
			return validation{
				Valid:  false,
				Reason: fmt.Sprintf("annotation cannot change: %q", staticAnnotationKey),
			}, nil
		}
	}

	return valid, nil
}

// Service extracts a service from an admission request
func toService(a []byte) (*corev1.Service, error) {
	p := corev1.Service{}
	if err := json.Unmarshal(a, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
