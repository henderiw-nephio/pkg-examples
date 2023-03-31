package mutator2

import (
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/example.com/foo/pkg/clustercontext/v1alpha1"
	"github.com/example.com/foo/pkg/fnruntime"
	interfacelibv1alpha1 "github.com/example.com/foo/pkg/interface/v1alpha1"
	ipallocv1v1alpha1 "github.com/example.com/foo/pkg/ipallocation/v1alpha1"
	nadlibv1 "github.com/example.com/foo/pkg/nad/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	fnruntime fnruntime.FnRuntime
	siteCode *string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}

	m.fnruntime = fnruntime.New(
		rl,
		&fnruntime.Config{
			For: fnruntime.ForConfig{
				ObjectRef: corev1.ObjectReference{
					APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
					Kind:       nephioreqv1alpha1.InterfaceKind,
				},
				PopulateFn: m.populateInterfaceFn,
			},
			Owns: map[corev1.ObjectReference]fnruntime.ConfigOperation{
				{
					APIVersion: nadv1.SchemeGroupVersion.Identifier(),
					Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
				}: fnruntime.ConfigOperationConditionOnly,
				{
					APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
					Kind:       ipamv1alpha1.IPAllocationKind,
				}: fnruntime.ConfigOperationDefault,
				// VLAN to be added as the 
				// NF Deployment to be added like the NAD -> this is a global iso per interface
			},
			Watch: map[corev1.ObjectReference]fnruntime.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.watchCallbackFn,
			},
			ConditionFn: m.populateConditionFn,
		},
	)
	m.fnruntime.Run()
	return true, nil
}

func (r *mutatorCtx) watchCallbackFn(o *fn.KubeObject) error {
	if o.GetAPIVersion() == infrav1alpha1.SchemeBuilder.GroupVersion.Identifier() && o.GetKind() == reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name() {
		clusterContext := clusterctxtlibv1alpha1.NewMutator(o.String())
		cluster, err := clusterContext.UnMarshal()
		if err != nil {
			return err
		}
		r.siteCode = cluster.Spec.SiteCode
	}
	return nil
}

func (r *mutatorCtx) populateConditionFn() bool {
	return r.siteCode != nil
}

func (r *mutatorCtx) populateInterfaceFn(o *fn.KubeObject) (map[corev1.ObjectReference]*fn.KubeObject, error) {
	resources := map[corev1.ObjectReference]*fn.KubeObject{}

	i := interfacelibv1alpha1.NewMutator(o.String())
	itfce, err := i.UnMarshal()
	if err != nil {
		return nil, err
	}

	// we assume right now that if the CNITYpe is not set this is a loopback interface
	if itfce.Spec.CNIType != "" {
		meta := metav1.ObjectMeta{
			Name: o.GetName(),
		}
		// ip allocation type network
		ipalloc := ipallocv1v1alpha1.NewGenerator(
			meta,
			ipamv1alpha1.IPAllocationSpec{
				PrefixKind:      ipamv1alpha1.PrefixKindNetwork,
				NetworkInstance: itfce.Spec.NetworkInstance.Name,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						//ipamv1alpha1.NephioSiteKey: *r.siteCode,
						"nephio.org/site": *r.siteCode,
					},
				},
			},
		)
		newObj, err := ipalloc.ParseKubeObject()
		if err != nil {
			return nil, err
		}

		resources[corev1.ObjectReference{
			APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
			Kind:       ipamv1alpha1.IPAllocationKind,
			Name:       o.GetName(),
		}] = newObj

		// allocate nad
		nad := nadlibv1.NewGenerator(
			meta,
			nadv1.NetworkAttachmentDefinitionSpec{},
		)
		newObj, err = nad.ParseKubeObject()
		if err != nil {
			return nil, err
		}
		resources[corev1.ObjectReference{
			APIVersion: nadv1.SchemeGroupVersion.Identifier(),
			Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
			Name:       o.GetName(),
		}] = newObj
	} else {
		// ip allocation type loopback
		ipalloc := ipallocv1v1alpha1.NewGenerator(
			metav1.ObjectMeta{
				Name: o.GetName(),
			},
			ipamv1alpha1.IPAllocationSpec{
				PrefixKind:      ipamv1alpha1.PrefixKindLoopback,
				NetworkInstance: itfce.Spec.NetworkInstance.Name,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						//ipamv1alpha1.NephioSiteKey: *r.siteCode,
						"nephio.org/site": *r.siteCode,
					},
				},
			},
		)
		newObj, err := ipalloc.ParseKubeObject()
		if err != nil {
			return nil, err
		}
		resources[corev1.ObjectReference{
			APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
			Kind:       ipamv1alpha1.IPAllocationKind,
			Name:       o.GetName(),
		}] = newObj
	}

	if itfce.Spec.AttachmentType == nephioreqv1alpha1.AttachmentTypeVLAN {
		// vlan allocation
	}
	return resources, nil
}
