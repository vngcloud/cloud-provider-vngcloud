// Package admission handles kubernetes admissions,
// it takes admission requests and returns admission reviews;
package admission

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

// Admitter is a container for admission business
type Admitter struct {
	Logger  *logrus.Entry
	Request *admissionv1.AdmissionRequest
}

// MutateReview takes an admission request and mutates the service within,
// it returns an admission review with mutations as a json patch (if any)
func (a Admitter) MutateReview() (*admissionv1.AdmissionReview, error) {
	service, err := a.Service()
	if err != nil {
		e := fmt.Sprintf("could not parse service in admission review request: %v", err)
		fmt.Println(e)
		return reviewResponse(a.Request.UID, false, http.StatusBadRequest, e), err
	}

	m := NewMutator(a.Logger)
	patch, err := m.MutatePatch(service)
	if err != nil {
		e := fmt.Sprintf("could not mutate service: %v", err)
		fmt.Println(e)
		return reviewResponse(a.Request.UID, false, http.StatusBadRequest, e), err
	}

	return patchReviewResponse(a.Request.UID, patch)
}

// MutateReview takes an admission request and validates the service within
// it returns an admission review
func (a Admitter) ValidateReview() (*admissionv1.AdmissionReview, error) {
	service, err := a.Service()
	if err != nil {
		e := fmt.Sprintf("could not parse service in admission review request: %v", err)
		return reviewResponse(a.Request.UID, false, http.StatusBadRequest, e), err
	}

	v := NewValidator(a.Logger)
	val, err := v.Validate(service)
	if err != nil {
		e := fmt.Sprintf("could not validate service: %v", err)
		return reviewResponse(a.Request.UID, false, http.StatusBadRequest, e), err
	}

	if !val.Valid {
		return reviewResponse(a.Request.UID, false, http.StatusForbidden, val.Reason), nil
	}

	return reviewResponse(a.Request.UID, true, http.StatusAccepted, "valid service"), nil
}

// Service extracts a service from an admission request
func (a Admitter) Service() (*corev1.Service, error) {
	if a.Request.Kind.Kind != "Service" {
		return nil, fmt.Errorf("only services are supported here")
	}

	p := corev1.Service{}
	if err := json.Unmarshal(a.Request.Object.Raw, &p); err != nil {
		return nil, err
	}

	return &p, nil
}

// reviewResponse TODO: godoc
func reviewResponse(uid types.UID, allowed bool, httpCode int32,
	reason string) *admissionv1.AdmissionReview {
	return &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: &admissionv1.AdmissionResponse{
			UID:     uid,
			Allowed: allowed,
			Result: &metav1.Status{
				Code:    httpCode,
				Message: reason,
			},
		},
	}
}

// patchReviewResponse builds an admission review with given json patch
func patchReviewResponse(uid types.UID, patch []byte) (*admissionv1.AdmissionReview, error) {
	patchType := admissionv1.PatchTypeJSONPatch

	return &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: &admissionv1.AdmissionResponse{
			UID:       uid,
			Allowed:   true,
			PatchType: &patchType,
			Patch:     patch,
		},
	}, nil
}
