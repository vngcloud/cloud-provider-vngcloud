package admission

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/ingress/controller"
	admissionv1 "k8s.io/api/admission/v1"
	nwv1 "k8s.io/api/networking/v1"
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
	if a.Kind.Kind != "Ingress" {
		return validation{
			Valid:  false,
			Reason: fmt.Sprintf("expected kind Ingress, got %q", a.Kind.Kind),
		}, nil
	}
	ingress, err := toIngress(a.Object.Raw)
	if err != nil {
		return validation{
			Valid:  false,
			Reason: fmt.Sprintf("could not parse ingress in admission review request: %v", err),
		}, err
	}
	if !controller.IsValid(ingress) {
		return valid, nil
	}

	// Prevent changing the load balancer name
	oldIngress := ingress
	if a.Operation == admissionv1.Update {
		oldIngress, err = toIngress(a.OldObject.Raw)
		if err != nil {
			return validation{
				Valid:  false,
				Reason: fmt.Sprintf("could not parse old ingress in admission review request: %v", err),
			}, err
		}
		if !controller.IsValid(oldIngress) {
			return valid, nil
		}

		staticAnnotationKey := "vks.vngcloud.vn/load-balancer-name"
		if oldIngress.Annotations[staticAnnotationKey] != ingress.Annotations[staticAnnotationKey] && oldIngress.Annotations[staticAnnotationKey] != "" {
			return validation{
				Valid:  false,
				Reason: fmt.Sprintf("annotation cannot change: %q", staticAnnotationKey),
			}, nil
		}
	}

	return valid, nil
}

// extracts from an admission request
func toIngress(a []byte) (*nwv1.Ingress, error) {
	p := nwv1.Ingress{}
	if err := json.Unmarshal(a, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
