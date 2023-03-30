package v1alpha1

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
)

func TestInterface(t *testing.T) {

	f := `apiVersion: req.nephio.org/v1alpha1
kind: Interface
metadata:
  name: n3
  annotations:
    config.kubernetes.io/local-config: "true"
spec:
  networkInstance:
    name: vpc-ran
  cniType: sriov
  attachmentType: vlan
`

	x := NewMutator(f)
	itfce, err := x.UnMarshal()
	if err != nil {
		t.Errorf("cannot unmarshal file: %s", err.Error())
	}

	cases := map[string]struct {
		fn   func(*nephioreqv1alpha1.Interface) string
		want string
	}{
		"NetworkInstanceName": {
			fn: func(itfce *nephioreqv1alpha1.Interface) string {
				return itfce.Spec.NetworkInstance.Name
			},
			want: "vpc-ran",
		},
		"CNITYpe": {
			fn: func(itfce *nephioreqv1alpha1.Interface) string {
				fmt.Println("spec", itfce.Spec)
				return string(itfce.Spec.CNIType)
			},
			want: "sriov",
		},
		"AttachementType": {
			fn: func(itfce *nephioreqv1alpha1.Interface) string {
				return string(itfce.Spec.AttachmentType)
			},
			want: "vlan",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.fn(itfce)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TestInterface: -want, +got:\n%s", diff)
			}
		})
	}
}
