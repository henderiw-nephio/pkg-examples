package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	ko "github.com/henderiw-nephio/pkg-examples/pkg/kubeobject"
	"github.com/nephio-project/nephio/krm-functions/lib/condkptsdk"
	vlanv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/vlan/v1alpha1"
	"github.com/nokia/k8s-ipam/pkg/proxy/clientproxy"
	"github.com/nokia/k8s-ipam/pkg/proxy/clientproxy/vlan"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	if err := fn.AsMain(fn.ResourceListProcessorFunc(Run)); err != nil {
		os.Exit(1)
	}
}

type fnCtx struct {
	sdk             condkptsdk.KptCondSDK
	VlanClientProxy clientproxy.Proxy[*vlanv1alpha1.VLANDatabase, *vlanv1alpha1.VLANAllocation]
}

func Run(rl *fn.ResourceList) (bool, error) {
	fnCtx := fnCtx{
		VlanClientProxy: vlan.NewMock(),
	}
	var err error
	fnCtx.sdk, err = condkptsdk.New(
		rl,
		&condkptsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: vlanv1alpha1.GroupVersion.Identifier(),
				Kind:       vlanv1alpha1.VLANAllocationKind,
			},
			PopulateOwnResourcesFn: nil,
			GenerateResourceFn:     fnCtx.updateVLANAllocationResource,
		},
	)
	if err != nil {
		rl.Results.ErrorE(err)
		return false, nil
	}
	return fnCtx.sdk.Run()
}

func (r *fnCtx) updateVLANAllocationResource(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	if forObj == nil {
		return nil, fmt.Errorf("expected a for object but got nil")
	}
	fn.Logf("ipalloc: %v\n", forObj)
	allocKOE, err := ko.NewFromKubeObject[*vlanv1alpha1.VLANAllocation](forObj)
	if err != nil {
		return nil, err
	}
	alloc, err := allocKOE.GetGoStruct()
	if err != nil {
		return nil, err
	}
	resp, err := r.VlanClientProxy.Allocate(context.Background(), alloc, nil)
	if err != nil {
		return nil, err
	}
	alloc.Status = resp.Status

	if alloc.Status.VLANID != nil {
		fn.Logf("alloc resp vlan: %v\n", *resp.Status.VLANID)
	}
	// set the status
	err = allocKOE.SetStatus(resp.Status)
	return &allocKOE.KubeObject, err
}
