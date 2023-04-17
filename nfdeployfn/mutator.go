package nfdeployfn

import (
	"fmt"
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/example.com/foo/pkg/clustercontext/v1alpha1"

	//ipallocv1v1alpha1 "github.com/example.com/foo/pkg/ipallocation/v1alpha1"
	kptcondsdk "github.com/example.com/foo/pkg/kpt-cond-sdk"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type mutatorCtx struct {
	fnCondSdk kptcondsdk.KptCondSDK
	siteCode  string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.fnCondSdk, err = kptcondsdk.New(
		rl,
		&kptcondsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: "dummy",
				Kind:       "NFDeployment",
			},
			Watch: map[corev1.ObjectReference]kptcondsdk.WatchCallbackFn{
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
			GenerateResourceFn:     m.generateResourceFn,
		},
	)
	if err != nil {
		rl.Results = append(rl.Results, fn.ErrorConfigObjectResult(err, nil))
	}
	m.fnCondSdk.Run()
	return true, nil
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

func (r *mutatorCtx) generateResourceFn(forObj *fn.KubeObject, objs []*fn.KubeObject) (*fn.KubeObject, error) {
	// just testing for now this should be removed
	if forObj != nil {
		return nil, fmt.Errorf("expected a nil for object")
	}

	// generate nfdeploy

	return nil, nil
}
