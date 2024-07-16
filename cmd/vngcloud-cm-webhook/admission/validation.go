package admission

import (
	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
)

// Validator is a container for mutation
type Validator struct {
	Logger *logrus.Entry
}

// NewValidator returns an initialised instance of Validator
func NewValidator(logger *logrus.Entry) *Validator {
	return &Validator{Logger: logger}
}

// itemValidators is an interface used to group functions mutating
type itemValidator interface {
	Validate(*admissionv1.AdmissionRequest) (validation, error)
	Name() string
}

type validation struct {
	Valid  bool
	Reason string
}

// Validate returns true if a item is valid
func (v *Validator) Validate(admission *admissionv1.AdmissionRequest) (validation, error) {
	// list of all validations to be applied
	validations := []itemValidator{
		commonValidator{v.Logger},
	}

	// apply all validations
	for _, v := range validations {
		var err error
		vp, err := v.Validate(admission)
		if err != nil {
			return validation{Valid: false, Reason: err.Error()}, err
		}
		if !vp.Valid {
			return validation{Valid: false, Reason: vp.Reason}, err
		}
	}

	return validation{Valid: true, Reason: "valid"}, nil
}
