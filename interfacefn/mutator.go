package interfacefn

import (
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	clusterctxtlibv1alpha1 "github.com/example.com/foo/pkg/clustercontext/v1alpha1"
	ipallocv1v1alpha1 "github.com/example.com/foo/pkg/ipallocation/v1alpha1"
	kptcondsdk "github.com/example.com/foo/pkg/kpt-cond-sdk"
	nadlibv1 "github.com/example.com/foo/pkg/nad/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	interfacelibv1alpha1 "github.com/nephio-project/nephio/krm-functions/lib/interface/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mutatorCtx struct {
	fnCondSdk       kptcondsdk.KptCondSDK
	siteCode        string
	masterInterface string
	cniType         string
}

func Run(rl *fn.ResourceList) (bool, error) {
	m := mutatorCtx{}
	var err error
	m.fnCondSdk, err = kptcondsdk.New(
		rl,
		&kptcondsdk.Config{
			For: corev1.ObjectReference{
				APIVersion: nephioreqv1alpha1.GroupVersion.Identifier(),
				Kind:       nephioreqv1alpha1.InterfaceKind,
			},
			Owns: map[corev1.ObjectReference]kptcondsdk.ResourceKind{
				{
					APIVersion: nadv1.SchemeGroupVersion.Identifier(),
					Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
				}: kptcondsdk.ResourceKindNone,
				{
					APIVersion: ipamv1alpha1.GroupVersion.Identifier(),
					Kind:       ipamv1alpha1.IPAllocationKind,
				}: kptcondsdk.ResourceKindFull,
				// VLAN to be added as the
				// NF Deployment to be added like the NAD -> this is a global iso per interface
			},
			Watch: map[corev1.ObjectReference]kptcondsdk.WatchCallbackFn{
				{
					APIVersion: infrav1alpha1.GroupVersion.Identifier(),
					Kind:       reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name(),
				}: m.ClusterContextCallbackFn,
			},
			PopulateOwnResourcesFn: m.populateInterfaceFn,
			GenerateResourceFn:     nil,
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
	r.masterInterface = cluster.Spec.CNIConfig.MasterInterface
	r.cniType = cluster.Spec.CNIConfig.CNIType
	return nil
}

func (r *mutatorCtx) populateInterfaceFn(o *fn.KubeObject) (map[corev1.ObjectReference]*fn.KubeObject, error) {
	resources := map[corev1.ObjectReference]*fn.KubeObject{}

	i, err := interfacelibv1alpha1.NewFromYAML([]byte(o.String()))
	if err != nil {
		return nil, err
	}

	// we assume right now that if the CNITYpe is not set this is a loopback interface
	if i.GetCNIType() != "" {
		meta := metav1.ObjectMeta{
			Name: o.GetName(),
		}
		// ip allocation type network
		ipalloc := ipallocv1v1alpha1.NewGenerator(
			meta,
			ipamv1alpha1.IPAllocationSpec{
				PrefixKind: ipamv1alpha1.PrefixKindNetwork,
				NetworkInstanceRef: &ipamv1alpha1.NetworkInstanceReference{
					Name: i.GetNetworkInstanceName(),
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						//ipamv1alpha1.NephioSiteKey: *r.siteCode,
						"nephio.org/site": r.siteCode,
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
				PrefixKind: ipamv1alpha1.PrefixKindLoopback,
				NetworkInstanceRef: &ipamv1alpha1.NetworkInstanceReference{
					Name: i.GetNetworkInstanceName(),
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						//ipamv1alpha1.NephioSiteKey: *r.siteCode,
						"nephio.org/site": r.siteCode,
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

	/*
		if itfce.Spec.AttachmentType == nephioreqv1alpha1.AttachmentTypeVLAN {
			// vlan allocation
		}
	*/
	return resources, nil
}
