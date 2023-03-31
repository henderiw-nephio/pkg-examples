package mutatordownstream

import (
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/example.com/foo/pkg/clustercontext/v1alpha1"
	"github.com/example.com/foo/pkg/fnruntime"
	nadlibv1 "github.com/example.com/foo/pkg/nad/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	fnruntime       fnruntime.FnRuntime
	masterInterface string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}

	m.fnruntime = fnruntime.NewDownstream(
		rl,
		&fnruntime.DownstreamRuntimeConfig{
			For: fnruntime.DownstreamRuntimeForConfig{
				ObjectRef: corev1.ObjectReference{
					APIVersion: nadv1.SchemeGroupVersion.Identifier(),
					Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
				},
				GenerateFn: m.generateNadFn,
			},
			Owns: map[corev1.ObjectReference]struct{}{
				{
					APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
					Kind:       ipamv1alpha1.IPAllocationKind,
				}: {},
				{
					APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
					Kind:       nephioreqv1alpha1.InterfaceKind,
				}: {},
			},
			Watch: map[corev1.ObjectReference]fnruntime.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.ClusterContextCallbackFn,
			},
		},
	)
	m.fnruntime.Run()
	return true, nil
}

func (r *mutatorCtx) ClusterContextCallbackFn(o *fn.KubeObject) error {
	if o.GetAPIVersion() == infrav1alpha1.SchemeBuilder.GroupVersion.Identifier() && o.GetKind() == reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name() {
		clusterContext := clusterctxtlibv1alpha1.NewMutator(o.String())
		cluster, err := clusterContext.UnMarshal()
		if err != nil {
			return err
		}
		r.masterInterface = cluster.Spec.CNIConfig.MasterInterface
	}
	return nil
}

func (r *mutatorCtx) generateNadFn(resources map[corev1.ObjectReference]fn.KubeObject) (*fn.KubeObject, error) {

	// loop throough resource get ip, vlan and masterInterface and generate a nad

	meta := metav1.ObjectMeta{
		Name: "dummyName",
	}
	for _, o := range resources {
		meta.Name = o.GetName()
	}

	return nadlibv1.NewGenerator(meta, nadv1.NetworkAttachmentDefinitionSpec{}).ParseKubeObject()
}
