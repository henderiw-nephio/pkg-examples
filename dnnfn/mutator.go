package dnnfn

import (
	"fmt"
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/clustercontext/v1alpha1"
	condkptsdk "github.com/henderiw-nephio/pkg-examples/pkg/condkptsdk"
	dnnlibv1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/dnn/v1alpha1"
	ipallocv1v1alpha1 "github.com/henderiw-nephio/pkg-examples/pkg/ipallocation/v1alpha1"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	allocv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/common/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/alloc/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	fnCondSdk condkptsdk.KptCondSDK
	siteCode  string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.fnCondSdk, err = condkptsdk.New(
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
			PopulateOwnResourcesFn: m.populateInterfaceFn,
			GenerateResourceFn:     m.generateResourceFn,
		},
	)
	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorConfigObjectResult(err, nil))
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
	return nil
}

func (r *mutatorCtx) populateInterfaceFn(o *fn.KubeObject) (fn.KubeObjects, error) {
	resources := fn.KubeObjects{}

	dnn := dnnlibv1alpha1.NewFromKubeObject(o)

	for _, pool := range dnn.GetPools() {
		alloc := ipamv1alpha1.BuildIPAllocation(
			metav1.ObjectMeta{
				//Name: o.GetName(),
				Name: fmt.Sprintf("%s-%s", o.GetName(), pool.Name),
			},
			ipamv1alpha1.IPAllocationSpec{
				PrefixKind: ipamv1alpha1.PrefixKindPool,
				NetworkInstance: &corev1.ObjectReference{
					Name: dnn.GetNetworkInstanceName(),
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						allocv1alpha1.NephioSiteKey: r.siteCode,
					},
				},
				PrefixLength: pool.PrefixLength,
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

func (r *mutatorCtx) generateResourceFn(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	// we expect a for object here
	if forObj == nil {
		return nil, fmt.Errorf("expected a for object but got nil")
	}
	for _, o := range objs {
		if o.GetAPIVersion() == ipamv1alpha1.GroupVersion.Identifier() && o.GetKind() == ipamv1alpha1.IPAllocationKind {
			alloc, _ := ipallocv1v1alpha1.NewFromKubeObject(o)
			prefix := alloc.GetAllocatedPrefix()

			forObj.SetAnnotation("prefix", prefix)
		}
	}
	return forObj, nil
}
