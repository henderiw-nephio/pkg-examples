package v1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func GetConditionType(o *corev1.ObjectReference) string {
	var sb strings.Builder
	sb.Reset()
	if o.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(o.APIVersion)
		if err == nil {
			sb.WriteString(gv.String())
		}
	}
	if o.Kind != "" {
		if sb.String() != "" {
			sb.WriteString(".")
		}
		sb.WriteString(o.Kind)
	}
	if o.Name != "" {
		if sb.String() != "" {
			sb.WriteString(".")
		}
		sb.WriteString(o.Name)
	}
	return sb.String()
}

func GetGVKNFromConditionType(ct string) (o *corev1.ObjectReference) {
	split := strings.Split(ct, "/")
	group := ""
	vkn := ct
	if len(split) > 1 {
		group = split[0]
		vkn = split[1]
	}
	newsplit := strings.Split(vkn, ".")
	if len(newsplit) == 3 {
		return &corev1.ObjectReference{
			APIVersion: fmt.Sprintf("%s/%s", group, newsplit[0]),
			Kind:       newsplit[1],
			Name:       newsplit[2],
		}
	}
	return &corev1.ObjectReference{}
}
