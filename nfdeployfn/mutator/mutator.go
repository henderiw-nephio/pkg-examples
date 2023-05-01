package mutator

import (
	"fmt"
	"reflect"

	ko "github.com/henderiw-nephio/pkg-examples/pkg/kubeobject"
	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	"github.com/nephio-project/nephio/krm-functions/lib/condkptsdk"
	corev1 "k8s.io/api/core/v1"
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
				APIVersion: "dummy",
				Kind:       "NFDeployment",
			},
			Watch: map[corev1.ObjectReference]condkptsdk.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.ClusterContextCallbackFn,
				{
					APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
					Kind:       nephioreqv1alpha1.InterfaceKind,
				}: nil,
				{
					APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
					Kind:       nephioreqv1alpha1.DataNetworkKind,
				}: nil,
			},
			PopulateOwnResourcesFn: nil,
			GenerateResourceFn:     m.updateNFDeployResource,
		},
	)
	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorConfigObjectResult(err, nil))
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
	return nil
}

func (r *mutatorCtx) updateNFDeployResource(forObj *fn.KubeObject, objs fn.KubeObjects) (*fn.KubeObject, error) {
	// just testing for now this should be removed
	if forObj != nil {
		return nil, fmt.Errorf("expected a nil for object")
	}

	// generate nfdeploy

	return nil, nil
}
