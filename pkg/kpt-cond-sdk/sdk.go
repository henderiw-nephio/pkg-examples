/*
Copyright 2023 The Nephio Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kptcondsdk

import (
	"fmt"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	"github.com/example.com/foo/pkg/kptrl"
	kptfilelibv1 "github.com/nephio-project/nephio/krm-functions/lib/kptfile/v1"
	corev1 "k8s.io/api/core/v1"
)

const FnRuntimeOwner = "fnruntime.nephio.org/owner"
const FnRuntimeDelete = "fnruntime.nephio.org/delete"

type KptCondSDK interface {
	Run()
}

type KptCondSDKConfig struct {
	For                    corev1.ObjectReference
	Owns                   map[corev1.ObjectReference]OwnKind         // ownkind distinguishes for condition only or condition and resourc generation
	Watch                  map[corev1.ObjectReference]WatchCallbackFn // used mainly for watches to non specific resources
	PopulateOwnResourcesFn PopulateOwnResourcesFn
	GenerateResourceFn     GenerateResourceFn
}

type PopulateOwnResourcesFn func(o *fn.KubeObject) (map[corev1.ObjectReference]*fn.KubeObject, error)
type GenerateResourceFn func([]*fn.KubeObject) (*fn.KubeObject, error)
type WatchCallbackFn func(o *fn.KubeObject) error

type OwnKind string

const (
	OwnKindConditionOnly      OwnKind = "conditionOnly"
	OwnKindConditionAndCreate OwnKind = "conditionAndCreate"
)

func New(rl *fn.ResourceList, cfg *KptCondSDKConfig) (KptCondSDK, error) {
	inv, err := newInventory(cfg)
	if err != nil {
		return nil, err
	}
	r := &kptcondsdk{
		cfg: cfg,
		inv: inv,
		rl:  kptrl.New(rl),
	}
	return r, nil
}

type kptcondsdk struct {
	cfg *KptCondSDKConfig
	inv Inventory
	rl  kptrl.ResourceList
}

func (r *kptcondsdk) Run() {
	r.populateInventory()
	r.populateChildren()
	r.updateChildren()
	r.generateResource()
}

// populateInventory populates the inventory with the conditions and resources
// related to the config
func (r *kptcondsdk) populateInventory() {
	// A forOwnerRef is an ownerReference associated to a for KRM resource
	// it is used to associated a watch KRM resource to the inventory
	// a watch KRM resource matching the forOwnerRef is assocatiated to the specific
	// for inventory ctx, otherwise the association happens to the global inventory context
	// The reason we do this is to make filtering more easy.
	var forOwnerRef *corev1.ObjectReference
	if r.rl.GetObjects().Len() > 0 {
		// we assume the kpt file is always resource idx 0 in the resourcelist
		o := r.rl.GetObjects()[0]

		kf, err := kptfilelibv1.New(o.String())
		if err != nil {
			fn.Log("error unmarshal kptfile during populateInventory")
			r.rl.AddResult(err, o)
		}

		// We first run through the conditions to check if an ownRef is associated
		// to the for resource objects. We call this the forOwnerRef
		// When a forOwnerRef exists it is used to associate a watch resource to the
		// inventory specific to the for resource or globally.
		for _, c := range kf.GetConditions() {
			// get the specific inventory context from the conditionType
			condRef := kptfilelibv1.GetGVKNFromConditionType(c.Type)
			// check if the conditionType is coming from a for KRM resource
			kindCtx, ok := r.inv.isGVKMatch(condRef)
			if ok && kindCtx.kind == forkind {
				// get the ownerRef from the consitionReason
				// to see if the forOwnerref is present and if so initialize the forOwnerRef using the GVK
				// information
				ownerRef := kptfilelibv1.GetGVKNFromConditionType(c.Reason)
				if ownerRef.Kind != "" {
					forOwnerRef = &corev1.ObjectReference{APIVersion: ownerRef.APIVersion, Kind: ownerRef.Kind}
				}
				// add the for resource to the inventory
				if err := r.inv.setExistingCondition(kindCtx, condRef, nil, &c); err != nil {
					fn.Logf("error setting exisiting condition to the inventory: %v\n", err.Error())
					r.rl.AddResult(err, o)
				}
			}
		}

		// Now we have the forOwnerRef we run through the condition again to populate the remaining
		// resources in the inventory
		for _, c := range kf.GetConditions() {
			condRef := kptfilelibv1.GetGVKNFromConditionType(c.Type)
			kindCtx, ok := r.inv.isGVKMatch(condRef)
			if !ok {
				continue
			}
			switch kindCtx.kind {
			case ownkind:
				// for owns it is possible that another fn/controller
				// owns this resource. we can check this by looking at the ownerRef
				// in the condition reason and check if this matches the forKind
				ownerRef := kptfilelibv1.GetGVKNFromConditionType(c.Reason)
				ownerKindCtx, ok := r.inv.isGVKMatch(ownerRef)
				if !ok || ownerKindCtx.kind != forkind {
					// this means the resource was added from a different kind so we dont need to add this
					continue
				}
				r.inv.setExistingCondition(kindCtx, ownerRef, condRef, &c)
			case watchkind:
				ownerRef := kptfilelibv1.GetGVKNFromConditionType(c.Reason)
				if forOwnerRef != nil && (ownerRef.APIVersion == forOwnerRef.APIVersion && ownerRef.Kind == forOwnerRef.Kind ||
					condRef.APIVersion == forOwnerRef.APIVersion && condRef.Kind == forOwnerRef.Kind) {
					// specific watch
					if err := r.inv.setExistingCondition(kindCtx, &corev1.ObjectReference{APIVersion: r.cfg.For.APIVersion, Kind: r.cfg.For.Kind, Name: condRef.Name}, condRef, &c); err != nil {
						fn.Logf("error setting exisiting condition to the inventory: %v\n", err.Error())
						r.rl.AddResult(err, o)
					}
				} else {
					if err := r.inv.setExistingCondition(kindCtx, condRef, nil, &c); err != nil {
						fn.Logf("error setting exisiting condition to the inventory: %v\n", err.Error())
						r.rl.AddResult(err, o)
					}
				}
			}
		}
	}
	for _, o := range r.rl.GetObjects() {
		// check if this resource matches our filters
		kindCtx, ok := r.inv.isGVKMatch(&corev1.ObjectReference{
			APIVersion: o.GetAPIVersion(),
			Kind:       o.GetKind(),
		})
		if !ok {
			continue
		}
		switch kindCtx.kind {
		case forkind:
			fn.Log("set existing for resource in inventory", &corev1.ObjectReference{
				APIVersion: o.GetAPIVersion(),
				Kind:       o.GetKind(),
				Name:       o.GetName(),
			}, o)
			if err := r.inv.setExistingResource(kindCtx, &corev1.ObjectReference{
				APIVersion: o.GetAPIVersion(),
				Kind:       o.GetKind(),
				Name:       o.GetName(),
			}, nil, o); err != nil {
				fn.Logf("error setting exisiting condition to the inventory: %v\n", err.Error())
				r.rl.AddResult(err, o)
			}
		case ownkind:
			// for owns it is possible that another fn/controller
			// owns this resource. we can check this by looking at the ownerRef
			// in the condition reason and check if this matches the forKind
			ownerRef := kptfilelibv1.GetGVKNFromConditionType(o.GetAnnotation(FnRuntimeOwner))
			ownerKindCtx, ok := r.inv.isGVKMatch(ownerRef)
			if !ok || ownerKindCtx.kind != forkind {
				// this means the resource was added from a different kind so we dont need to add this
				continue
			}
			fn.Log("set existing own resource in inventory", ownerRef, o)
			if err := r.inv.setExistingResource(kindCtx,
				&corev1.ObjectReference{
					APIVersion: ownerRef.APIVersion,
					Kind:       ownerRef.Kind,
					Name:       ownerRef.Name,
				}, &corev1.ObjectReference{
					APIVersion: o.GetAPIVersion(),
					Kind:       o.GetKind(),
					Name:       o.GetName(),
				}, o); err != nil {
				fn.Logf("error setting exisiting resource to the inventory: %v\n", err.Error())
				r.rl.AddResult(err, o)
			}
		case watchkind:
			ownerRef := kptfilelibv1.GetGVKNFromConditionType(o.GetAnnotation(FnRuntimeOwner))
			if forOwnerRef != nil && (ownerRef.APIVersion == forOwnerRef.APIVersion && ownerRef.Kind == forOwnerRef.Kind ||
				o.GetAPIVersion() == forOwnerRef.APIVersion && o.GetKind() == forOwnerRef.Kind) {
				// specific watch
				r.inv.setExistingResource(kindCtx,
					&corev1.ObjectReference{
						APIVersion: r.cfg.For.APIVersion,
						Kind:       r.cfg.For.Kind,
						Name:       o.GetName(),
					}, &corev1.ObjectReference{
						APIVersion: o.GetAPIVersion(),
						Kind:       o.GetKind(),
						Name:       o.GetName(),
					}, o)
			} else {
				fn.Log("set existing watch resource in inventory", &corev1.ObjectReference{
					APIVersion: o.GetAPIVersion(),
					Kind:       o.GetKind(),
					Name:       o.GetName(),
				}, o)
				r.inv.setExistingResource(kindCtx, &corev1.ObjectReference{
					APIVersion: o.GetAPIVersion(),
					Kind:       o.GetKind(),
					Name:       o.GetName(),
				}, nil, o)
			}
		}
	}
}

func (r *kptcondsdk) populateChildren() {
	watches, ready := r.inv.isReady()
	if !ready {
		return
	}
	// call the watch callbacks to provide info to the fns in a genric way they dont have to parse
	// to understand which resource we are trying to associate
	for _, w := range watches {
		fn.Logf("run watch: %v\n", w.o)
		if w.callback != nil {
			if err := w.callback(w.o); err != nil {
				fn.Log("populatechildren not ready: watch callback failed: %v\n", err.Error())
				r.rl.AddResult(err, w.o)
				ready = false
			}
		}
	}
	if ready {
		for forRef, forObj := range r.inv.getForResources() {
			fn.Log("PopulateOwnResourcesFn", forObj)
			if r.cfg.PopulateOwnResourcesFn != nil {
				res, err := r.cfg.PopulateOwnResourcesFn(forObj)
				if err != nil {
					fn.Log("error populating new resource: %v", err.Error())
					r.rl.AddResult(err, forObj)
				} else {
					for objRef, newObj := range res {
						kc, _ := r.inv.isGVKMatch(&corev1.ObjectReference{APIVersion: objRef.APIVersion, Kind: objRef.Kind})
						fn.Logf("populate new resource: forRef %v objRef %v kc: %v\n", forRef, objRef, kc)

						// set owner reference on the new resource
						newObj.SetAnnotation(FnRuntimeOwner, kptfilelibv1.GetConditionType(&forRef))
						// add the resource to the existing list
						r.inv.setNewResource(kc,
							&forRef,
							&objRef,
							newObj)
					}
				}
			}
		}
	}
}

func (r *kptcondsdk) updateChildren() {
	// get the kpt file
	kf, err := kptfilelibv1.New(r.rl.GetObjects()[0].String())
	if err != nil {
		fn.Log("error parsing kptfile")
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}

	// perform a diff to validate the existing resource against the new resources
	diff, err := r.inv.diff()
	if err != nil {
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	fn.Logf("diff: %v\n", diff)

	// if the fn is not ready to act we stop immediately
	_, ok := r.inv.isReady()
	if !ok {
		// delete all child resources by setting the annotation and set the condition to false
		for _, obj := range diff.deleteObjs {
			fn.Logf("delete set condition: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			// set condition
			c := kptv1.Condition{
				Type:    kptfilelibv1.GetConditionType(&obj.ref),
				Status:  kptv1.ConditionFalse,
				Reason:  fmt.Sprintf("%s.%s", kptfilelibv1.GetConditionType(&r.cfg.For), obj.ref.Name),
				Message: "not ready",
			}
			kf.SetConditions(c)
			// update the status back in the inventory
			r.inv.setExistingCondition(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&c)
			// set the delete annotation
			obj.obj.SetAnnotation(FnRuntimeDelete, "true")
			r.rl.SetObject(&obj.obj)
			// update the status back in the inventory
			r.inv.setExistingResource(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&obj.obj)
		}
		// we cannot return as we still have to update the kptfile
	} else {
		// act upon the diff
		// update conditions
		for _, obj := range diff.createConditions {
			fn.Logf("create condition: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			// create condition again
			c := kptv1.Condition{
				Type:    kptfilelibv1.GetConditionType(&obj.ref),
				Status:  kptv1.ConditionFalse,
				Reason:  fmt.Sprintf("%s.%s", kptfilelibv1.GetConditionType(&r.cfg.For), obj.ref.Name),
				Message: "create condition again as it was deleted",
			}
			kf.SetConditions(c)
			// update the status back in the inventory
			r.inv.setExistingCondition(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&c)
		}
		for _, obj := range diff.deleteConditions {
			fn.Logf("delete condition: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			// delete condition
			kf.DeleteCondition(kptfilelibv1.GetConditionType(&obj.ref))
			// update the status back in the inventory
			r.inv.deleteExistingCondition(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref)
		}

		// update resources
		for _, obj := range diff.createObjs {
			fn.Logf("create obj: ref: %s, ownkind: %s\n", kptfilelibv1.GetConditionType(&obj.ref), obj.ownKind)
			// create obj/condition - add resource to resource list
			c := kptv1.Condition{
				Type:    kptfilelibv1.GetConditionType(&obj.ref),
				Status:  kptv1.ConditionFalse,
				Reason:  fmt.Sprintf("%s.%s", kptfilelibv1.GetConditionType(&r.cfg.For), obj.ref.Name),
				Message: "create new resource",
			}
			kf.SetConditions(c)
			// update the status back in the inventory
			r.inv.setExistingCondition(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&c)

			if obj.ownKind == OwnKindConditionAndCreate {
				r.rl.SetObject(&obj.obj)
				// update the status back in the inventory
				r.inv.setExistingResource(&kindCtx{kind: ownkind},
					&obj.forRef,
					&obj.ref,
					&obj.obj)

			}
		}
		for _, obj := range diff.updateObjs {
			fn.Logf("update obj: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			// update condition - add resource to resource list
			c := kptv1.Condition{
				Type:    kptfilelibv1.GetConditionType(&obj.ref),
				Status:  kptv1.ConditionFalse,
				Reason:  fmt.Sprintf("%s.%s", kptfilelibv1.GetConditionType(&r.cfg.For), obj.ref.Name),
				Message: "update existing resource",
			}
			kf.SetConditions(c)
			// update the status back in the inventory
			r.inv.setExistingCondition(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&c)

			if obj.ownKind == OwnKindConditionAndCreate {
				r.rl.SetObject(&obj.obj)
				// update the status back in the inventory
				r.inv.setExistingResource(&kindCtx{kind: ownkind},
					&obj.forRef,
					&obj.ref,
					&obj.obj)
			}
		}
		for _, obj := range diff.deleteObjs {
			fn.Logf("delete obj: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			// create condition - add resource to resource list
			c := kptv1.Condition{
				Type:    kptfilelibv1.GetConditionType(&obj.ref),
				Status:  kptv1.ConditionFalse,
				Reason:  fmt.Sprintf("%s.%s", kptfilelibv1.GetConditionType(&r.cfg.For), obj.ref.Name),
				Message: "delete existing resource",
			}
			kf.SetConditions(c)
			// update the status back in the inventory
			r.inv.setExistingCondition(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&c)
			// update resource to resoucelist with delete Timestamp set
			obj.obj.SetAnnotation(FnRuntimeDelete, "true")
			r.rl.SetObject(&obj.obj)
			// update the status back in the inventory
			r.inv.setExistingResource(&kindCtx{kind: ownkind},
				&obj.forRef,
				&obj.ref,
				&obj.obj)
		}

	}
	// update the kptfile with the latest consitions
	kptfile, err := kf.ParseKubeObject()
	if err != nil {
		fn.Log(err)
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	r.rl.SetObject(kptfile)

}

// updateResource updates the resource and when complete sets the condition
// to true
func (r *kptcondsdk) generateResource() {
	// get the kpt file
	kf, err := kptfilelibv1.New(r.rl.GetObjects()[0].String())
	if err != nil {
		fn.Log("error parsing kptfile")
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}

	watches, ready := r.inv.isReady()
	if !ready {
		// when the overal status is not ready delete all resources
		// TBD if we need to check the delete annotation
		readyMap := r.inv.getResourceReadyMap()
		for _, readyCtx := range readyMap {
			if readyCtx.ForObj != nil {
				if len(r.cfg.Owns) == 0 {
					r.rl.DeleteObject(readyCtx.ForObj)
				}
			}
		}
		return
	}
	// we call the global watches
	for _, w := range watches {
		if w.callback != nil {
			if err := w.callback(w.o); err != nil {
				fn.Log("populatechildren not ready: watch callback failed: %v", err.Error())
				r.rl.AddResult(err, w.o)
				ready = false
			}
		}
	}
	// the overall status is ready, so lets check the readiness map
	readyMap := r.inv.getResourceReadyMap()
	for forRef, readyCtx := range readyMap {
		// if the for is not ready delete the object
		if !readyCtx.Ready {
			if readyCtx.ForObj != nil {
				if len(r.cfg.Owns) == 0 {
					r.rl.DeleteObject(readyCtx.ForObj)
				}
			}
			continue
		}

		if r.cfg.GenerateResourceFn != nil {
			objs := []*fn.KubeObject{}
			for _, o := range readyCtx.Owns {
				objs = append(objs, &o)
			}
			for _, o := range readyCtx.Watches {
				objs = append(objs, &o)
			}

			newObj, err := r.cfg.GenerateResourceFn(objs)
			if err != nil {
				fn.Log("error generating new resource: %v", err.Error())
				r.rl.AddResult(err, readyCtx.ForObj)
			} else {
				// TODO set it based on the condition
				// set owner reference on the new resource
				newObj.SetAnnotation(FnRuntimeOwner, kptfilelibv1.GetConditionType(&forRef))
				// add the resource to the kptfile
				r.rl.SetObject(newObj)

				// TODO set conditions
				//kf.SetConditions()
			}
		}
	}
	// update the kptfile with the latest consitions
	kptfile, err := kf.ParseKubeObject()
	if err != nil {
		fn.Log(err)
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	r.rl.SetObject(kptfile)
}
