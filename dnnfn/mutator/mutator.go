package mutator

import (
	"fmt"
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	ko "github.com/henderiw-nephio/pkg-examples/pkg/kubeobject"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	"github.com/nephio-project/nephio/krm-functions/lib/condkptsdk"
	allocv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/common/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	sdk      condkptsdk.KptCondSDK
	siteCode string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.sdk, err = condkptsdk.New(
		rl,
		&condkptsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
				Kind:       nephioreqv1alpha1.DataNetworkKind,
			},
			Owns: map[corev1.ObjectReference]condkptsdk.ResourceKind{
				{
					APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
					Kind:       ipamv1alpha1.IPAllocationKind,
				}: condkptsdk.ChildRemote,
			},
			Watch: map[corev1.ObjectReference]condkptsdk.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.ClusterContextCallbackFn,
			},
			PopulateOwnResourcesFn: m.desiredOwnedResourceList,
			GenerateResourceFn:     m.updateDnnResource,
		},
	)
	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorConfigObjectResult(err, nil))
	}
	return m.sdk.Run()
}

// ClusterContextCallbackFn provides a callback for the cluster context
// resources in the resourceList
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
	return nil
}

func (r *mutatorCtx) desiredOwnedResourceList(o *fn.KubeObject) (fn.KubeObjects, error) {
	resources := fn.KubeObjects{}

	dnnKOE, err := ko.NewFromKubeObject[nephioreqv1alpha1.DataNetwork](o)
	if err != nil {
		return nil, err
	}
	dnn, err := dnnKOE.GetGoStruct()
	if err != nil {
		return nil, err
	}

	for _, pool := range dnn.Spec.Pools {
		alloc := ipamv1alpha1.BuildIPAllocation(
			metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s", o.GetName(), pool.Name),
			},
			ipamv1alpha1.IPAllocationSpec{
				Kind:            ipamv1alpha1.PrefixKindPool,
				NetworkInstance: dnn.Spec.NetworkInstance,
				AllocationLabels: allocv1alpha1.AllocationLabels{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							allocv1alpha1.NephioSiteKey: r.siteCode,
						},
					},
				},
				PrefixLength: &pool.PrefixLength,
			},
			ipamv1alpha1.IPAllocationStatus{},
		)
		o, err := fn.NewFromTypedObject(alloc)
		if err != nil {
			return nil, err
		}

		resources = append(resources, o)
	}
	return resources, nil
}

func (r *mutatorCtx) updateDnnResource(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	// we expect a for object here
	if forObj == nil {
		return nil, fmt.Errorf("expected a for object but got nil")
	}
	dnnKOE, err := ko.NewFromKubeObject[nephioreqv1alpha1.DataNetwork](forObj)
	if err != nil {
		return nil, err
	}
	dnn, err := dnnKOE.GetGoStruct()
	if err != nil {
		return nil, err
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
		// todo update pool status
		dnn.Status.Pools = append(dnn.Status.Pools, nephioreqv1alpha1.PoolStatus{Name: alloc.GetName(), IPAllocation: allocGoStruct.Status})

	}
	err = dnnKOE.SetFromTypedObject(dnn)
	return &dnnKOE.KubeObject, err
}
