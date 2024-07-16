package admission

import (
	"encoding/json"

	"github.com/sirupsen/logrus"
	"github.com/wI2L/jsondiff"
	corev1 "k8s.io/api/core/v1"
)

// Mutator is a container for mutation
type Mutator struct {
	Logger *logrus.Entry
}

// NewMutator returns an initialised instance of Mutator
func NewMutator(logger *logrus.Entry) *Mutator {
	return &Mutator{Logger: logger}
}

// itemMutator is an interface used to group functions mutating
type itemMutator interface {
	Mutate(*corev1.Service) (*corev1.Service, error)
	Name() string
}

// MutatePatch returns a json patch containing all the mutations needed for
// a given service
func (m *Mutator) MutatePatch(service *corev1.Service) ([]byte, error) {
	// var serviceName string
	// if service.Name != "" {
	// 	serviceName = service.Name
	// } else {
	// 	if service.ObjectMeta.GenerateName != "" {
	// 		serviceName = service.ObjectMeta.GenerateName
	// 	}
	// }
	// log := logrus.WithField("service_name", serviceName)

	// list of all mutations to be applied to the service
	mutations := []itemMutator{
		// minLifespanTolerations{Logger: log},
		// injectEnv{Logger: log},
	}

	mservice := service.DeepCopy()

	// apply all mutations
	for _, m := range mutations {
		var err error
		mservice, err = m.Mutate(mservice)
		if err != nil {
			return nil, err
		}
	}

	// generate json patch
	patch, err := jsondiff.Compare(service, mservice)
	if err != nil {
		return nil, err
	}

	patchb, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	return patchb, nil
}
