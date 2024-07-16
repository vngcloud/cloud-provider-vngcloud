package admission

import (
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
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
	Validate(*corev1.Service) (validation, error)
	Name() string
}

type validation struct {
	Valid  bool
	Reason string
}

// Validate returns true if a service is valid
func (v *Validator) Validate(service *corev1.Service) (validation, error) {
	var serviceName string
	if service.Name != "" {
		serviceName = service.Name
	} else {
		if service.ObjectMeta.GenerateName != "" {
			serviceName = service.ObjectMeta.GenerateName
		}
	}
	log := logrus.WithField("service_name", serviceName)
	log.Print("delete me")

	// list of all validations to be applied to the service
	validations := []itemValidator{
		nameValidator{v.Logger},
	}

	// apply all validations
	for _, v := range validations {
		var err error
		vp, err := v.Validate(service)
		if err != nil {
			return validation{Valid: false, Reason: err.Error()}, err
		}
		if !vp.Valid {
			return validation{Valid: false, Reason: vp.Reason}, err
		}
	}

	return validation{Valid: true, Reason: "valid service"}, nil
}
