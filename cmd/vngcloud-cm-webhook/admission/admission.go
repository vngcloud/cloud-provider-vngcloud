// Package admission handles kubernetes admissions,
// it takes admission requests and returns admission reviews;
package admission

import (
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

// Admitter is a container for admission business
type Admitter struct {
	Logger  *logrus.Entry
	Request *admissionv1.AdmissionRequest
}

// ValidateReview takes an admission request and validates the item within
// it returns an admission review
func (a Admitter) ValidateReview() (*admissionv1.AdmissionReview, error) {

	v := NewValidator(a.Logger)
	val, err := v.Validate(a.Request)
	if err != nil {
		e := fmt.Sprintf("could not validate: %v", err)
		return reviewResponse(a.Request.UID, false, http.StatusBadRequest, e), err
	}

	if !val.Valid {
		return reviewResponse(a.Request.UID, false, http.StatusForbidden, val.Reason), nil
	}

	return reviewResponse(a.Request.UID, true, http.StatusAccepted, "valid"), nil
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
