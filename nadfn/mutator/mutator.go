package mutator

import (
	"fmt"
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/clustercontext/v1alpha1"

	//ipallocv1v1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/ipallocation/v1alpha1"
	condkptsdk "github.com/henderiw-nephio/pkg-examples/pkg/condkptsdk"
	ipalloclibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/ipallocation/v1alpha1"
	nadlibv1 "github.com/henderiw-nephio/pkg-examples/pkg/nad/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	fnCondSdk       condkptsdk.KptCondSDK
	masterInterface string
	cniType         string
	siteCode        string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.fnCondSdk, err = condkptsdk.New(
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
			},
			PopulateOwnResourcesFn: nil,
			GenerateResourceFn:     m.generateResourceFn,
		},
	)
	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorResult(err))
	}
	return m.fnCondSdk.Run()
}

func (r *mutatorCtx) ClusterContextCallbackFn(o *fn.KubeObject) error {
	clusterContext := clusterctxtlibv1alpha1.NewMutator(o.String())
	cluster, err := clusterContext.UnMarshal()
	if err != nil {
		return err
	}
	if cluster.Spec.CNIConfig.MasterInterface == "" {
		return fmt.Errorf("MasterInterface on ClusterContext cannot be empty")
	} else {
		r.masterInterface = cluster.Spec.CNIConfig.MasterInterface
	}
	if cluster.Spec.CNIConfig.CNIType == "" {
		return fmt.Errorf("CNIType on ClusterContext cannot be empty")
	} else {
		r.cniType = cluster.Spec.CNIConfig.CNIType
	}
	if cluster.Spec.SiteCode == nil {
		return fmt.Errorf("SiteCode on ClusterContext cannot be empty")
	} else {
		r.siteCode = *cluster.Spec.SiteCode
	}
	return nil
}

func (r *mutatorCtx) generateResourceFn(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	if len(objs) == 0 {
		return nil, fmt.Errorf("expecting sone object to generate the nad")
	}
	// generate an empty nad struct
	meta := metav1.ObjectMeta{Name: objs[0].GetName()}
	nad, err := nadlibv1.NewFromGoStruct(nadlibv1.BuildNetworkAttachementDefinition(
		meta,
		nadv1.NetworkAttachmentDefinitionSpec{
			Config: "",
		},
	))
	if err != nil {
		return nil, err
	}

	fn.Logf("cniType: %s, masterInterface: %s\n", r.cniType, r.masterInterface)
	nad.SetCNIType(r.cniType) // cniType should come from interface
	nad.SetNadMaster(r.masterInterface)

	ipallocs := objs.Where(fn.IsGroupVersionKind(ipamv1alpha1.IPAllocationGroupVersionKind))
	for _, ipalloc := range ipallocs {

		alloc, err := ipalloclibv1alpha1.NewFromKubeObject(ipalloc)
		if err != nil {
			return nil, err
		}
		allocGoStruct, err := alloc.GetGoStruct()
		if err != nil {
			return nil, err
		}
		nad.SetIpamAddress([]nadlibv1.Addresses{{
			Address: allocGoStruct.Status.AllocatedPrefix,
			Gateway: allocGoStruct.Status.Gateway,
		}})
	}

	return &nad.KubeObject, nil
}
