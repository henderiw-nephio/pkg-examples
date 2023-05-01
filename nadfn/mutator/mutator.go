package mutator

import (
	"fmt"
	"reflect"

	ko "github.com/henderiw-nephio/pkg-examples/pkg/kubeobject"
	"github.com/nephio-project/nephio/krm-functions/lib/condkptsdk"
	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/ipam/v1alpha1"
	vlanv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/vlan/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	sdk             condkptsdk.KptCondSDK
	masterInterface string
	cniType         string
	siteCode        string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.sdk, err = condkptsdk.New(
		rl,
		&condkptsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: nadv1.SchemeGroupVersion.Identifier(),
				Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
			},
			Watch: map[corev1.ObjectReference]condkptsdk.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.ClusterContextCallbackFn,
				{
					APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
					Kind:       ipamv1alpha1.IPAllocationKind,
				}: nil,
				{
					APIVersion: vlanv1alpha1.GroupVersion.Identifier(),
					Kind:       vlanv1alpha1.VLANAllocationKind,
				}: nil,
				{
					APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
					Kind:       nephioreqv1alpha1.InterfaceKind,
				}: nil,
			},
			PopulateOwnResourcesFn: nil,
			GenerateResourceFn:     m.updateNadResource,
		},
	)
	if err != nil {
		rl.Results.ErrorE(err)
		return false, nil
	}
	return m.sdk.Run()
}

func (r *mutatorCtx) ClusterContextCallbackFn(o *fn.KubeObject) error {
	clusterKOE, err := ko.NewFromKubeObject[*infrav1alpha1.ClusterContext](o)
	if err != nil {
		return err
	}
	clusterContext, err := clusterKOE.GetGoStruct()
	if err != nil {
		return err
	}
	if clusterContext.Spec.SiteCode == nil {
		return fmt.Errorf("mandatory field `siteCode` is missing from ClusterContext %q", clusterContext.Name)
	}
	if r.siteCode != "" && r.siteCode != *clusterContext.Spec.SiteCode {
		return fmt.Errorf("multiple ClusterContext objects with confliciting `siteCode` fields found in the package")
	}
	r.siteCode = *clusterContext.Spec.SiteCode
	if clusterContext.Spec.CNIConfig == nil {
		return fmt.Errorf("mandatory field `cniConfig` is missing from ClusterContext %q", clusterContext.Name)
	}
	if (r.masterInterface != "" && clusterContext.Spec.CNIConfig.MasterInterface != r.masterInterface) ||
		(r.cniType != "" && clusterContext.Spec.CNIConfig.CNIType != r.cniType) {
		return fmt.Errorf("multiple ClusterContext objects with confliciting `cniConfig` fields found in the package")
	}
	r.masterInterface = clusterContext.Spec.CNIConfig.MasterInterface
	r.cniType = clusterContext.Spec.CNIConfig.CNIType
	return nil
}

func (r *mutatorCtx) updateNadResource(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	if len(objs) == 0 {
		return nil, fmt.Errorf("expecting sone object to generate the nad")
	}
	// generate an empty nad struct
	meta := metav1.ObjectMeta{Name: objs[0].GetName()}
		
	itfces := objs.Where(fn.IsGroupVersionKind(nephioreqv1alpha1.InterfaceGroupVersionKind))
	for _, itfce := range itfces {
		ifce, err := ko.NewFromKubeObject[*nephioreqv1alpha1.Interface](itfce)
		if err != nil {
			return nil, err
		}
		itfceGoStruct, err := ifce.GetGoStruct()
		if err != nil {
			return nil, err
		}
		// set CNI Type
		fn.Log(itfceGoStruct)
	}
	ipallocs := objs.Where(fn.IsGroupVersionKind(ipamv1alpha1.IPAllocationGroupVersionKind))
	for _, ipalloc := range ipallocs {
		alloc, err := ko.NewFromKubeObject[*ipamv1alpha1.IPAllocation](ipalloc)
		if err != nil {
			return nil, err
		}
		allocGoStruct, err := alloc.GetGoStruct()
		if err != nil {
			return nil, err
		}
		// set IP
		fn.Log(allocGoStruct)
	}
	vlanallocs := objs.Where(fn.IsGroupVersionKind(ipamv1alpha1.IPAllocationGroupVersionKind))
	for _, vlanalloc := range vlanallocs {
		alloc, err := ko.NewFromKubeObject[*vlanv1alpha1.VLANAllocation](vlanalloc)
		if err != nil {
			return nil, err
		}
		allocGoStruct, err := alloc.GetGoStruct()
		if err != nil {
			return nil, err
		}
		// set VLAN
		fn.Log(allocGoStruct)
	}

	return r.getNAD(meta, nadv1.NetworkAttachmentDefinitionSpec{})
}

func (r *mutatorCtx) getNAD(meta metav1.ObjectMeta, spec nadv1.NetworkAttachmentDefinitionSpec) (*fn.KubeObject, error) {
	nad := BuildNetworkAttachmentDefinition(
		meta,
		spec,
	)
	return fn.NewFromTypedObject(nad)
}

func BuildNetworkAttachmentDefinition(meta metav1.ObjectMeta, spec nadv1.NetworkAttachmentDefinitionSpec) *nadv1.NetworkAttachmentDefinition {
	return &nadv1.NetworkAttachmentDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nadv1.SchemeGroupVersion.Identifier(),
			Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
		},
		ObjectMeta: meta,
		Spec:       spec,
	}
}