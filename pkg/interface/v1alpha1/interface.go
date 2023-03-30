package v1alpha1

import (
	"errors"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	"sigs.k8s.io/yaml"
)

type Interface interface {
	// Unmarshal decodes the raw document within the in byte slice and assigns decoded values into the out value.
	// it leverages the  "sigs.k8s.io/yaml" library
	UnMarshal() (*nephioreqv1alpha1.Interface, error)
	// Marshal serializes the value provided into a YAML document based on "sigs.k8s.io/yaml".
	// The structure of the generated document will reflect the structure of the value itself.
	Marshal() ([]byte, error)
	// ParseKubeObject returns a fn sdk KubeObject; if something failed an error
	// is returned
	ParseKubeObject() (*fn.KubeObject, error)
}

// NewMutator creates a new mutator for the interface
// It expects a raw byte slice as input representing the serialized yaml file
func NewMutator(b string) Interface {
	return &itfce{
		raw: []byte(b),
	}
}

type itfce struct {
	raw   []byte
	itfce *nephioreqv1alpha1.Interface
}

func (r *itfce) UnMarshal() (*nephioreqv1alpha1.Interface, error) {
	i := &nephioreqv1alpha1.Interface{}
	if err := yaml.Unmarshal(r.raw, i); err != nil {
		return nil, err
	}
	r.itfce = i
	return i, nil
}

// Marshal serializes the value provided into a YAML document based on "sigs.k8s.io/yaml".
// The structure of the generated document will reflect the structure of the value itself.
func (r *itfce) Marshal() ([]byte, error) {
	if r.itfce == nil {
		return nil, errors.New("cannot marshal unitialized interface")
	}
	b, err := yaml.Marshal(r.itfce)
	if err != nil {
		return nil, err
	}
	r.raw = b
	return b, err
}

// ParseKubeObject returns a fn sdk KubeObject; if something failed an error
// is returned
func (r *itfce) ParseKubeObject() (*fn.KubeObject, error) {
	b, err := r.Marshal()
	if err != nil {
		return nil, err
	}
	return fn.ParseKubeObject(b)
}
