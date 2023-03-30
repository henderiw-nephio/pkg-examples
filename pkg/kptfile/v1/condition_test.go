package v1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

func TestGetConditionType(t *testing.T) {
	type object struct {
		apiVersion string
		kind       string
		name       string
		dummy      string
	}

	tests := map[string]struct {
		input object
		want  string
	}{
		"ConditionType": {
			input: object{
				apiVersion: "x.y/z",
				kind:       "b",
				name:       "c",
			},
			want: "x.y/z.b.c",
		},
		"ConditionTypeEmptyApiVersion": {
			input: object{
				kind: "b",
				name: "c",
			},
			want: "b.c",
		},
		"ConditionTypeWrongAPIVersion": {
			input: object{
				apiVersion: "a.a",
				kind:       "b",
				name:       "c",
			},
			want: "a.a.b.c",
		},
		"ConditionTypeName": {
			input: object{
				name: "c",
			},
			want: "c",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := GetConditionType(&corev1.ObjectReference{
				APIVersion: tc.input.apiVersion,
				Kind:       tc.input.kind,
				Name:       tc.input.name,
				Namespace:  tc.input.dummy,
			})
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TestGetConditionType GVK: -want, +got:\n%s", diff)
			}

		})
	}
}

func TestGetGVKNFromConditionType(t *testing.T) {

	tests := map[string]struct {
		t    string
		want *corev1.ObjectReference
	}{
		"GVKN": {
			t:    "a.b/c.d.e",
			want: &corev1.ObjectReference{APIVersion: "a.b/c", Kind: "d", Name: "e"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := GetGVKNFromConditionType(tc.t)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TestGetGVKNFromConditionType GVK: -want, +got:\n%s", diff)
			}

		})
	}
}
