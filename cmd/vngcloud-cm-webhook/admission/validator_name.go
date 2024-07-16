package admission

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// nameValidator is a container for validating the name
type nameValidator struct {
	Logger logrus.FieldLogger
}

// nameValidator implements the itemValidator interface
var _ itemValidator = (*nameValidator)(nil)

// Name returns the name of nameValidator
func (n nameValidator) Name() string {
	return "name_validator"
}

// Validate inspects the name of a given item and returns validation.
// The returned validation is only valid if the item name does not contain some
// bad string.
func (n nameValidator) Validate(service *corev1.Service) (validation, error) {
	badString := "offensive"

	if strings.Contains(service.Name, badString) {
		v := validation{
			Valid:  false,
			Reason: fmt.Sprintf("service name contains %q", badString),
		}
		return v, nil
	}

	return validation{Valid: true, Reason: "valid name"}, nil
}
