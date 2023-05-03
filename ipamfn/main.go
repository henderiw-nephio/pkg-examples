package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	ko "github.com/henderiw-nephio/pkg-examples/pkg/kubeobject"
	"github.com/nephio-project/nephio/krm-functions/lib/condkptsdk"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/ipam/v1alpha1"
	"github.com/nokia/k8s-ipam/pkg/proxy/clientproxy"
	"github.com/nokia/k8s-ipam/pkg/proxy/clientproxy/ipam"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	r := &FnR{
		IpamClientProxy: ipam.NewMock(),
	}

	if err := fn.AsMain(fn.ResourceListProcessorFunc(r.Run)); err != nil {
		os.Exit(1)
	}
}

type FnR struct {
	IpamClientProxy clientproxy.Proxy[*ipamv1alpha1.NetworkInstance, *ipamv1alpha1.IPAllocation]
}

func (r *FnR) Run(rl *fn.ResourceList) (bool, error) {
	sdk, err := condkptsdk.New(
		rl,
		&condkptsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
				Kind:       ipamv1alpha1.IPAllocationKind,
			},
			PopulateOwnResourcesFn: nil,
			GenerateResourceFn:     r.updateIPAllocationResource,
		},
	)
	if err != nil {
		rl.Results.ErrorE(err)
		return false, nil
	}
	return sdk.Run()
}

func (r *FnR) updateIPAllocationResource(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	if forObj == nil {
		return nil, fmt.Errorf("expected a for object but got nil")
	}
	fn.Logf("ipalloc: %v\n", forObj)
	allocKOE, err := ko.NewFromKubeObject[*ipamv1alpha1.IPAllocation](forObj)
	if err != nil {
		return nil, err
	}
	alloc, err := allocKOE.GetGoStruct()
	if err != nil {
		return nil, err
	}
	resp, err := r.IpamClientProxy.Allocate(context.Background(), alloc, nil)
	if err != nil {
		return nil, err
	}
	alloc.Status = resp.Status

	if alloc.Status.Prefix != nil {
		fn.Logf("ipalloc resp prefix: %v\n", *resp.Status.Prefix)
	}
	if alloc.Status.Gateway != nil {
		fn.Logf("ipalloc resp gateway: %v\n", *resp.Status.Gateway)
	}
	// set the status
	//err = allocKOE.SetStatus(resp.Status)
	err = allocKOE.SetFromTypedObject(resp)
	return &allocKOE.KubeObject, err
}
