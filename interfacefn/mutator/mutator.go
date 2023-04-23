package mutator

import (
	"fmt"
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/clustercontext/v1alpha1"
	condkptsdk "github.com/henderiw-nephio/pkg-examples/pkg/condkptsdk"
	interfacelibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/interface/v1alpha1"
	ipalloclibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/ipallocation/v1alpha1"
	nadlibv1 "github.com/henderiw-nephio/pkg-examples/pkg/nad/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	allocv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/common/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	fnCondSdk       condkptsdk.KptCondSDK
	siteCode        string
	masterInterface string
	cniType         string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.fnCondSdk, err = condkptsdk.New(
		rl,
		&condkptsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
				Kind:       nephioreqv1alpha1.InterfaceKind,
			},
			Owns: map[corev1.ObjectReference]condkptsdk.ResourceKind{
				{
					APIVersion: nadv1.SchemeGroupVersion.Identifier(),
					Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
				}: condkptsdk.ChildRemoteCondition,
				{
					APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
					Kind:       ipamv1alpha1.IPAllocationKind,
				}: condkptsdk.ChildRemote,
				// VLAN to be added as the
				// NF Deployment to be added like the NAD -> this is a global iso per interface
			},
			Watch: map[corev1.ObjectReference]condkptsdk.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.ClusterContextCallbackFn,
			},
			PopulateOwnResourcesFn: m.populateFn,
			GenerateResourceFn:     m.generateFn,
		},
	)
	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorResult(err))
		return false, err
	}
	return m.fnCondSdk.Run()
}

func (r *mutatorCtx) ClusterContextCallbackFn(o *fn.KubeObject) error {
	clusterContext := clusterctxtlibv1alpha1.NewMutator(o.String())
	cluster, err := clusterContext.UnMarshal()
	if err != nil {
		return err
	}
	r.siteCode = *cluster.Spec.SiteCode
	r.masterInterface = cluster.Spec.CNIConfig.MasterInterface
	r.cniType = cluster.Spec.CNIConfig.CNIType
	return nil
}

func (r *mutatorCtx) populateFn(o *fn.KubeObject) (fn.KubeObjects, error) {
	resources := fn.KubeObjects{}

	itfce, err := interfacelibv1alpha1.NewFromKubeObject(o)
	if err != nil {
		return nil, err
	}

	// we assume right now that if the CNITYpe is not set this is a loopback interface
	if itfce.GetCNIType() != "" {
		meta := metav1.ObjectMeta{
			Name: o.GetName(),
		}
		// ip allocation type network
		alloc := ipamv1alpha1.BuildIPAllocation(
			meta,
			ipamv1alpha1.IPAllocationSpec{
				PrefixKind: ipamv1alpha1.PrefixKindNetwork,
				NetworkInstance: &corev1.ObjectReference{
					Name: itfce.GetNetworkInstanceName(),
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						allocv1alpha1.NephioSiteKey: r.siteCode,
					},
				},
			},
			ipamv1alpha1.IPAllocationStatus{},
		)

		o, err := fn.NewFromTypedObject(alloc)
		if err != nil {
			return nil, err
		}

		resources = append(resources, o)

		// allocate nad
		nad := nadlibv1.BuildNetworkAttachementDefinition(
			meta,
			nadv1.NetworkAttachmentDefinitionSpec{},
		)
		o, err = fn.NewFromTypedObject(nad)
		if err != nil {
			return nil, err
		}
		resources = append(resources, o)

	} else {
		// ip allocation type loopback
		alloc := ipamv1alpha1.BuildIPAllocation(
			metav1.ObjectMeta{
				Name: o.GetName(),
			},
			ipamv1alpha1.IPAllocationSpec{
				PrefixKind: ipamv1alpha1.PrefixKindLoopback,
				NetworkInstance: &corev1.ObjectReference{
					Name: itfce.GetNetworkInstanceName(),
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						allocv1alpha1.NephioSiteKey: r.siteCode,
					},
				},
			},
			ipamv1alpha1.IPAllocationStatus{},
		)
		o, err := fn.NewFromTypedObject(alloc)
		if err != nil {
			return nil, err
		}

		resources = append(resources, o)

	}

	/*
		if itfce.Spec.AttachmentType == nephioreqv1alpha1.AttachmentTypeVLAN {
			// vlan allocation
		}
	*/
	return resources, nil
}

func (r *mutatorCtx) generateFn(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	if forObj == nil {
		return nil, fmt.Errorf("expected a for object but got nil")
	}
	itfce, err := interfacelibv1alpha1.NewFromKubeObject(forObj)
	if err != nil {
		return nil, err
	}

	ipallocs := objs.Where(fn.IsGroupVersionKind(ipamv1alpha1.IPAllocationGroupVersionKind))
	for _, ipalloc := range ipallocs {
		if ipalloc.GetName() == forObj.GetName() {
			alloc, err := ipalloclibv1alpha1.NewFromKubeObject(ipalloc)
			if err != nil {
				return nil, err
			}
			allocGoStruct, err := alloc.GetGoStruct()
			if err != nil {
				return nil, err
			}
			if err := itfce.SetIPAllocationStatus(&allocGoStruct.Status); err != nil {
				return nil, err
			}
		}
	}
	return &itfce.KubeObject, nil
}
