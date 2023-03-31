package fnruntime

import (
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	kptfilelibv1 "github.com/example.com/foo/pkg/kptfile/v1"
	"github.com/example.com/foo/pkg/kptrl"
	corev1 "k8s.io/api/core/v1"
)

type DownstreamRuntimeConfig struct {
	For   DownstreamRuntimeForConfig
	Watch map[corev1.ObjectReference]WatchCallbackFn
}

type DownstreamRuntimeForConfig struct {
	ObjectRef  corev1.ObjectReference
	GenerateFn GenerateFn
}

func NewDownstream(rl *fn.ResourceList, c *DownstreamRuntimeConfig) FnRuntime {
	r := &downstreamFnRuntime{
		rl:         kptrl.New(rl),
		inv:        NewDownstreamInventory(),
		cfg:        c,
		owners:     map[string]struct{}{},
		forObjects: map[corev1.ObjectReference]*fn.KubeObject{},
	}
	return r
}

type downstreamFnRuntime struct {
	cfg        *DownstreamRuntimeConfig
	rl         kptrl.ResourceList
	inv        DownstreamInventory
	owners     map[string]struct{}
	forObjects map[corev1.ObjectReference]*fn.KubeObject
}

func (r *downstreamFnRuntime) Run() {
	r.initialize()
	r.update()
}

func (r *downstreamFnRuntime) initialize() {
	// First check if the for resource is wildcard or not;
	// The inventory is populated based on wildcard status
	if r.rl.GetObjects().Len() > 0 {
		// we assume the kpt file is always resource idx 0 in the resourcelist
		o := r.rl.GetObjects()[0]

		kf := kptfilelibv1.NewMutator(o.String())
		var err error
		if _, err = kf.UnMarshal(); err != nil {
			fn.Log("error unmarshal kptfile in initialize")
			r.rl.AddResult(err, o)
		}

		// populate condition inventory
		for _, c := range kf.GetConditions() {
			// based on the ForObj determine if there is work to be done
			if strings.Contains(c.Type, kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef)) {
				if c.Status == kptv1.ConditionTrue {
					if c.Reason == "wildcard" {
						r.inv.AddForCondition(corev1.ObjectReference{}, &c)
					} else {
						// c.reason contains the ownerRef
						r.owners[c.Reason] = struct{}{}
						r.inv.AddForCondition(*kptfilelibv1.GetGVKNFromConditionType(c.Reason), &c)
					}
				}
			}
		}

		// collect conditions
		for _, c := range kf.GetConditions() {
			objRef := *kptfilelibv1.GetGVKNFromConditionType(c.Type)
			if len(r.owners) == 0 && r.inv.GetForConditionStatus(corev1.ObjectReference{}) { // this is a wildcard
				// for wildcard the owner reference is a dummy unitialized owner reference
				r.inv.AddCondition(corev1.ObjectReference{}, objRef, &c)
			} else {
				// r.owners implicitly tell us that work is required
				if _, ok := r.owners[c.Reason]; ok {
					r.inv.AddCondition(*kptfilelibv1.GetGVKNFromConditionType(c.Reason), *kptfilelibv1.GetGVKNFromConditionType(c.Type), &c)
				}
			}
		}
	}
	// filter the related objects in case of no wildcard
	// if there is a wildcard we just provide all the resource to the generator Fn since all resources might be relevant
	if len(r.owners) != 0 {
		for _, o := range r.rl.GetObjects() {
			// add all resources that were generated based on owner reference
			if _, ok := r.owners[o.GetAnnotation(FnRuntimeOwner)]; ok {
				if o.GetAPIVersion() == r.cfg.For.ObjectRef.APIVersion && o.GetKind() == r.cfg.For.ObjectRef.Kind {
					r.inv.AddForResource(*kptfilelibv1.GetGVKNFromConditionType(o.GetAnnotation(FnRuntimeOwner)), o)
				} else {
					r.inv.AddResource(*kptfilelibv1.GetGVKNFromConditionType(o.GetAnnotation(FnRuntimeOwner)), corev1.ObjectReference{
						APIVersion: o.GetAPIVersion(),
						Kind:       o.GetKind(),
						Name:       o.GetName(),
					}, o)
				}
			}

			// add the owner object to the list
			if _, ok := r.owners[kptfilelibv1.GetConditionType(&corev1.ObjectReference{
				APIVersion: o.GetAPIVersion(),
				Kind:       o.GetKind(),
				Name:       o.GetName(),
			})]; ok {
				// add the owner object to the list in case relevant data is needed from there
				r.inv.AddResource(corev1.ObjectReference{
					APIVersion: o.GetAPIVersion(),
					Kind:       o.GetKind(),
					Name:       o.GetName(),
				}, corev1.ObjectReference{
					APIVersion: o.GetAPIVersion(),
					Kind:       o.GetKind(),
					Name:       o.GetName(),
				}, o)
			}

			// the watch object should provide generic information to the generator
			for objRef, watchCallbackFn := range r.cfg.Watch {
				if o.GetAPIVersion() == objRef.APIVersion &&
					o.GetKind() == objRef.Kind {
					// provide watch resource
					if err := watchCallbackFn(o); err != nil {
						r.rl.AddResult(err, o)
					}
				}
			}
		}
	} else {
		// wildcard
		if r.inv.GetForConditionStatus(corev1.ObjectReference{}) {
			for _, o := range r.rl.GetObjects() {
				r.inv.AddResource(corev1.ObjectReference{}, corev1.ObjectReference{
					APIVersion: o.GetAPIVersion(),
					Kind:       o.GetKind(),
					Name:       o.GetName(),
				}, o)
			}
		}
	}
}

func (r *downstreamFnRuntime) update() {
	kf := kptfilelibv1.NewMutator(r.rl.GetObjects()[0].String())
	var err error
	if _, err = kf.UnMarshal(); err != nil {
		fn.Log("error unmarshal kptfile")
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}

	for _, readyCtx := range r.inv.IsReady() {
		if readyCtx.Ready {
			// generate the obj irrespective of current status
			if r.cfg.For.GenerateFn != nil {
				o, err := r.cfg.For.GenerateFn(readyCtx.Objs)
				if err != nil {
					r.rl.AddResult(err, o)
				} else {
					if readyCtx.ForObj != nil {
						r.rl.SetObject(o)
					} else {
						r.rl.AddObject(o)
					}
				}
			}
		} else {
			// if obj exists delete it
			if readyCtx.ForCondition != nil {
				kf.DeleteCondition(readyCtx.ForCondition.Type)
			}
			if readyCtx.ForObj != nil {
				r.rl.DeleteObject(readyCtx.ForObj)
			}
		}
	}
}
