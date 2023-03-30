package mutator

import (
	"reflect"
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	clusterctxtlibv1alpha1 "github.com/example.com/foo/pkg/clustercontext/v1alpha1"
	interfacelibv1alpha1 "github.com/example.com/foo/pkg/interface/v1alpha1"
	"github.com/example.com/foo/pkg/inventory"
	ipallocv1v1alpha1 "github.com/example.com/foo/pkg/ipallocation/v1alpha1"
	kptfilelibv1 "github.com/example.com/foo/pkg/kptfile/v1"
	"github.com/example.com/foo/pkg/kptrl"
	nadlibv1 "github.com/example.com/foo/pkg/nad/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nephioreqv1alpha1 "github.com/nephio-project/api/nf_requirements/v1alpha1"
	infrav1alpha1 "github.com/nephio-project/nephio-controller-poc/apis/infra/v1alpha1"
	ipamv1alpha1 "github.com/nokia/k8s-ipam/apis/ipam/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type mutatorCtx struct {
	childResources []schema.GroupVersionKind
	inventory      inventory.Inventory

	siteCode *string
	rl       kptrl.ResourceList
}

var ipamGVK = schema.GroupVersionKind{Group: ipamv1alpha1.GroupVersion.Group, Version: ipamv1alpha1.GroupVersion.Version, Kind: ipamv1alpha1.IPAllocationKind}

// TO BE UPDATED with real vlan allocation CRD
var vlanGVK = schema.GroupVersionKind{Group: ipamv1alpha1.GroupVersion.Group, Version: ipamv1alpha1.GroupVersion.Version, Kind: ipamv1alpha1.IPAllocationKind}
var nadGVK = schema.GroupVersionKind{Group: nadv1.SchemeGroupVersion.Group, Version: nadv1.SchemeGroupVersion.Version, Kind: reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name()}

func Run(rl *fn.ResourceList) (bool, error) {
	// initialize the mutator Ctx
	m := mutatorCtx{
		childResources: []schema.GroupVersionKind{ipamGVK, vlanGVK, nadGVK},
		inventory:      inventory.New(),
		rl:             kptrl.New(rl),
	}

	m.initialize()
	m.populate()
	m.update()

	return true, nil
}

// initialize walks over the resource list and adds the existing resources in the inventory
// -> kptfile - capture exisitng conditions in the package kptfile
// -> nad - capture existing KRM in the package
// -> ipAllocation - capture existing KRM in the package
// -> vlanAllocation - capture existing KRM in the package
// The clustercontext is captured
func (r *mutatorCtx) initialize() {
	for _, o := range r.rl.GetObjects() {
		if o.GetAPIVersion() == kptv1.KptFileGVK().GroupVersion().String() && o.GetKind() == kptv1.KptFileName {
			kf := kptfilelibv1.NewMutator(o.String())
			var err error
			if _, err = kf.UnMarshal(); err != nil {
				fn.Log("error unmarshal kptfile in initialize")
				r.rl.AddResult(err, o)
			}

			// populate condition inventory
			for _, gvk := range r.childResources {
				for _, c := range kf.GetConditions() {
					if strings.Contains(c.Type, gvk.String()) {
						r.inventory.AddExistingCondition(kptfilelibv1.GetGVKNFromConditionType(c.Type), &c)
					}
				}
			}
		}

		for _, gvk := range r.childResources {
			if o.GetAPIVersion() == gvk.GroupVersion().String() && o.GetKind() == gvk.Kind {
				r.inventory.AddExistingResource(&corev1.ObjectReference{
					APIVersion: gvk.GroupVersion().String(),
					Kind:       gvk.Kind,
					Name:       o.GetName(),
				}, o)
			}
		}

		/*
			if o.GetAPIVersion() == ipamv1alpha1.GroupVersion.Identifier() && o.GetKind() == ipamv1alpha1.IPAllocationKind {
				// populate inventory index
				r.inventory.AddExistingResource(&corev1.ObjectReference{
					APIVersion: ipamGVK.GroupVersion().String(),
					Kind:       ipamGVK.Kind,
					Name:       o.GetName(),
				}, o)
			}

				// TODO VLAN
				//if o.GetAPIVersion() == ipamv1alpha1.GroupVersion.Identifier() && o.GetKind() == ipamv1alpha1.IPAllocationKind {
				//	fn.Log("ipallocation", o.GetName())
				//}
			if o.GetAPIVersion() == nadv1.SchemeGroupVersion.Identifier() && o.GetKind() == reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name() {
				// populate inventory index
				r.inventory.AddExistingResource(&corev1.ObjectReference{
					APIVersion: nadGVK.GroupVersion().String(),
					Kind:       nadGVK.Kind,
					Name:       o.GetName(),
				}, o)
			}
		*/
		if o.GetAPIVersion() == infrav1alpha1.SchemeBuilder.GroupVersion.Identifier() && o.GetKind() == reflect.TypeOf(infrav1alpha1.ClusterContext{}).Name() {
			clusterContext := clusterctxtlibv1alpha1.NewMutator(o.String())
			cluster, err := clusterContext.UnMarshal()
			if err != nil {
				r.rl.AddResult(err, o)
			}
			r.siteCode = cluster.Spec.SiteCode
		}
	}
}

// populate populates the inventory with the new resources
// - nad, ipAllocation, vlanAllocation based on the requirements set in the interface CR(s)
func (r *mutatorCtx) populate() {
	if r.siteCode != nil {
		for _, o := range r.rl.GetObjects() {
			if o.GetAPIVersion() == nephioreqv1alpha1.GroupVersion.Identifier() && o.GetKind() == nephioreqv1alpha1.InterfaceKind {
				i := interfacelibv1alpha1.NewMutator(o.String())
				itfce, err := i.UnMarshal()
				if err != nil {
					r.rl.AddResult(err, o)
				}

				// we assume right now that is the CNITYpe is not set this is a loopback interface
				if itfce.Spec.CNIType != "" {
					meta := metav1.ObjectMeta{
						Name: o.GetName(),
					}
					// ip allocation type network
					ipalloc := ipallocv1v1alpha1.NewGenerator(
						meta,
						ipamv1alpha1.IPAllocationSpec{
							PrefixKind:      ipamv1alpha1.PrefixKindNetwork,
							NetworkInstance: itfce.Spec.NetworkInstance.Name,
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									//ipamv1alpha1.NephioSiteKey: *r.siteCode,
									"nephio.org/site": *r.siteCode,
								},
							},
						},
					)
					newObj, err := ipalloc.ParseKubeObject()
					if err != nil {
						r.rl.AddResult(err, o)
					}
					r.inventory.AddNewResource(&corev1.ObjectReference{
						APIVersion: ipamGVK.GroupVersion().String(),
						Kind:       ipamGVK.Kind,
						Name:       o.GetName(),
					}, newObj)
					// allocate nad
					nad := nadlibv1.NewGenerator(
						meta,
						nadv1.NetworkAttachmentDefinitionSpec{},
					)
					newObj, err = nad.ParseKubeObject()
					if err != nil {
						r.rl.AddResult(err, o)
					}
					r.inventory.AddNewResource(&corev1.ObjectReference{
						APIVersion: nadGVK.GroupVersion().String(),
						Kind:       nadGVK.Kind,
						Name:       o.GetName(),
					}, newObj)
				} else {
					// ip allocation type loopback
					ipalloc := ipallocv1v1alpha1.NewGenerator(
						metav1.ObjectMeta{
							Name: o.GetName(),
						},
						ipamv1alpha1.IPAllocationSpec{
							PrefixKind:      ipamv1alpha1.PrefixKindLoopback,
							NetworkInstance: itfce.Spec.NetworkInstance.Name,
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									//ipamv1alpha1.NephioSiteKey: *r.siteCode,
									"nephio.org/site": *r.siteCode,
								},
							},
						},
					)
					newObj, err := ipalloc.ParseKubeObject()
					if err != nil {
						r.rl.AddResult(err, o)
					}
					r.inventory.AddNewResource(&corev1.ObjectReference{
						APIVersion: ipamGVK.GroupVersion().String(),
						Kind:       ipamGVK.Kind,
						Name:       o.GetName(),
					}, newObj)
				}

				if itfce.Spec.AttachmentType == nephioreqv1alpha1.AttachmentTypeVLAN {
					// vlan allocation
				}
			}
		}
	}
}

// update performs a diff on the inventory and performs the respective operations
// on the resources
// create/delete/update conditions in the kptfile
// create/update the CR, for delete the deletetimestamp is set on the CR such that the
// respective controllers/functions take care of the resource deletion
func (r *mutatorCtx) update() {
	// kptfile
	kf := kptfilelibv1.NewMutator(r.rl.GetObjects()[0].String())
	var err error
	if _, err = kf.UnMarshal(); err != nil {
		fn.Log("error unmarshal kptfile")
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}

	// perform a diff
	diff, err := r.inventory.Diff()
	if err != nil {
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}

	if r.siteCode == nil {
		// set deletion timestamp on all resources
		for _, obj := range diff.DeleteObjs {
			fn.Logf("create set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// set condition
			kf.SetConditions(kptv1.Condition{
				Type:   strings.ReplaceAll(kptfilelibv1.GetConditionType(&obj.Ref), "/", "_"),
				Status: kptv1.ConditionFalse,
				Reason: "cluster context has no site id",
			})
			// update the release timestamp
			r.rl.SetObjectWithDeleteTimestamp(&obj.Obj)
		}
		return
	} else {
		for _, obj := range diff.CreateConditions {
			fn.Logf("create condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// create condition again
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: "create condition again as it was deleted",
			})
		}
		for _, obj := range diff.DeleteConditions {
			fn.Logf("delete condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// delete condition
			kf.DeleteCondition(strings.ReplaceAll(kptfilelibv1.GetConditionType(&obj.Ref), "/", "_"))
		}
		for _, obj := range diff.CreateObjs {
			fn.Logf("create set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// create condition - add resource to resource list
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: "create new resource",
			})
			// for NAD(s) we dont need to update the resource as the information is not available
			// vlan/ip need to be allocated
			if obj.Ref.APIVersion != nadGVK.GroupVersion().String() && obj.Ref.Name != nadGVK.Kind {
				// add resource to resoucelist
				r.rl.AddObject(&obj.Obj)
			}
		}
		for _, obj := range diff.UpdateObjs {
			fn.Logf("update set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// update condition - add resource to resource list
			kf.SetConditions(kptv1.Condition{
				Type:   strings.ReplaceAll(kptfilelibv1.GetConditionType(&obj.Ref), "/", "_"),
				Status: kptv1.ConditionFalse,
				Reason: "update existing resource",
			})
			// for NAD(s) we dont need to update the resource as the information is not available
			// vlan/ip need to be allocated
			if obj.Ref.APIVersion != nadGVK.GroupVersion().String() && obj.Ref.Name != nadGVK.Kind {
				// update resource to resoucelist
				r.rl.SetObject(&obj.Obj)
			}

		}
		for _, obj := range diff.DeleteObjs {
			fn.Logf("update set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// create condition - add resource to resource list
			kf.SetConditions(kptv1.Condition{
				Type:   strings.ReplaceAll(kptfilelibv1.GetConditionType(&obj.Ref), "/", "_"),
				Status: kptv1.ConditionFalse,
				Reason: "delete existing resource",
			})
			// update resource to resoucelist with delete Timestamp set
			r.rl.SetObjectWithDeleteTimestamp(&obj.Obj)
		}
	}

	kptfile, err := kf.ParseKubeObject()
	if err != nil {
		fn.Log(err)
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	r.rl.SetObject(kptfile)
}
