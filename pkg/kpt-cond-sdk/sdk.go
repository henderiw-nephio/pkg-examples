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
type ResourceKind string

const (
	// ResourceKindNone defines a GVK resource for which only conditions need to be created
	ResourceKindNone ResourceKind = "none"
	// ResourceKindFull defines a GVK resource for which conditions and resources need to be created
	ResourceKindFull ResourceKind = "full"
)

type Config struct {
	For                    corev1.ObjectReference
	Owns                   map[corev1.ObjectReference]ResourceKind    // ResourceKind distinguishes ResourceKindNone and ResourceKindFull
	Watch                  map[corev1.ObjectReference]WatchCallbackFn // Used for watches to non specific resources
	PopulateOwnResourcesFn PopulateOwnResourcesFn
	GenerateResourceFn     GenerateResourceFn
}

type PopulateOwnResourcesFn func(*fn.KubeObject) (map[corev1.ObjectReference]*fn.KubeObject, error)

// the list of objects contains the owns and the specific watches
type GenerateResourceFn func([]*fn.KubeObject) (*fn.KubeObject, error)
type WatchCallbackFn func(*fn.KubeObject) error

func New(rl *fn.ResourceList, cfg *Config) (KptCondSDK, error) {
	inv, err := newInventory(cfg)
	if err != nil {
		return nil, err
	}
	r := &sdk{
		cfg: cfg,
		inv: inv,
		rl:  kptrl.New(rl),
	}
	return r, nil
}

type sdk struct {
	cfg  *Config
	inv  Inventory
	rl   *kptrl.ResourceList
	kptf kptfilelibv1.KptFile
}

func (r *sdk) Run() {
	if r.rl.GetObjects().Len() == 0 {
		r.rl.AddResult(fmt.Errorf("no resources present in the resourcelist"), nil)
		return
	}
	// get the kptfile first as we need it in various places
	// we assume the kpt file is always resource idx 0 in the resourcelist
	var err error
	r.kptf, err = kptfilelibv1.New(r.rl.GetObjects()[0].String())
	if err != nil {
		fn.Log("error unmarshal kptfile during populateInventory")
		r.rl.AddResult(err, r.rl.GetObjects()[0])
		return
	}

	// stage 1 of the sdk pipeline
	r.populateInventory()
	r.populateChildren()
	r.updateChildren()
	// stage 2 of the sdk pipeline
	r.generateResource()

}

// populateInventory populates the inventory with the conditions and resources
// related to the config
func (r *sdk) populateInventory() {
	// To make filtering easier the inventory distinguishes global resources
	// versus specific resources associated to a forInstance (specified through the SDK Config).
	// To perform this filtering we use the concept of the forOwnerRef, which is
	// an ownerReference associated to the forGVK
	// A watchedResource matching the forOwnerRef is assocatiated to the specific
	// forInventory context. If no match was found to the forOwnerRef the watchedResource is associated
	// to the global context
	var forOwnerRef *corev1.ObjectReference
	// we assume the kpt file is always resource idx 0 in the resourcelist, the object is used
	// as a reference to errors when we encounter issues with the condition processing
	// since conditions are stored in the kptFile
	o := r.rl.GetObjects()[0]

	// We first run through the conditions to check if an ownRef is associated
	// to the for resource objects. We call this the forOwnerRef
	// When a forOwnerRef exists it is used to associate a watch resource to the
	// inventory specific to the for resource or globally.
	for _, c := range r.kptf.GetConditions() {
		// get the specific inventory context from the conditionType
		ref := kptfilelibv1.GetGVKNFromConditionType(c.Type)
		// check if the conditionType is coming from a for KRM resource
		kindCtx, ok := r.inv.isGVKMatch(ref)
		if ok && kindCtx.gvkKind == forGVKKind {
			// get the ownerRef from the consitionReason
			// to see if the forOwnerref is present and if so initialize the forOwnerRef using the GVK
			// information
			ownerRef := kptfilelibv1.GetGVKNFromConditionType(c.Reason)
			if err := validateGVKRef(*ownerRef); err == nil {
				forOwnerRef = &corev1.ObjectReference{APIVersion: ownerRef.APIVersion, Kind: ownerRef.Kind}
			}
		}
	}
	// Now we have the forOwnerRef we run through the condition again to populate the remaining
	// resources in the inventory
	for _, c := range r.kptf.GetConditions() {
		ref := kptfilelibv1.GetGVKNFromConditionType(c.Type)
		ownerRef := kptfilelibv1.GetGVKNFromConditionType(c.Reason)
		r.populate(forOwnerRef, ref, ownerRef, c, o)
	}
	for _, o := range r.rl.GetObjects() {
		ref := &corev1.ObjectReference{
			APIVersion: o.GetAPIVersion(),
			Kind:       o.GetKind(),
			Name:       o.GetName(),
		}
		ownerRef := kptfilelibv1.GetGVKNFromConditionType(o.GetAnnotation(FnRuntimeOwner))
		r.populate(forOwnerRef, ref, ownerRef, o, o)
	}

}

func (r *sdk) populate(forOwnerRef, ref, ownerRef *corev1.ObjectReference, x any, o *fn.KubeObject) {
	// we lookup in the GVK context we initialized in the beginning to validate
	// if the gvk is relevant for this fn/controller
	// what the gvk Kind is about through the kindContext
	gvkKindCtx, ok := r.inv.isGVKMatch(getGVKRefFromGVKNref(ref))
	if !ok {
		// it can be that a resource in the kpt package is not relevant for this fn/controller
		// As such we return
		return
	}
	fn.Logf("set existing object in inventory, kind %s, ref: %v ownerRef: %v\n", gvkKindCtx.gvkKind, ref, ownerRef)
	switch gvkKindCtx.gvkKind {
	case forGVKKind:
		if err := r.inv.set(gvkKindCtx, []corev1.ObjectReference{*ref}, x, false); err != nil {
			fn.Logf("error setting exisiting object in the inventory: %v\n", err.Error())
			r.rl.AddResult(err, o)
		}
	case ownGVKKind:
		ownerKindCtx, ok := r.inv.isGVKMatch(ownerRef)
		if !ok || ownerKindCtx.gvkKind != forGVKKind {
			// this means the resource was added from a different kind
			// we dont need to add this to the inventory
			return
		}
		if err := r.inv.set(gvkKindCtx, []corev1.ObjectReference{*ownerRef, *ref}, x, false); err != nil {
			fn.Logf("error setting exisiting resource to the inventory: %v\n", err.Error())
			r.rl.AddResult(err, o)
		}
	case watchGVKKind:
		// check if the watch is specific or global
		// if no forOwnerRef is set the watch is global
		// if a forOwnerref is set we check if either the ownerRef or ref is match the GVK
		// the specifics of the name is sorted out later
		if forOwnerRef != nil && (ownerRef.APIVersion == forOwnerRef.APIVersion && ownerRef.Kind == forOwnerRef.Kind ||
			ref.APIVersion == forOwnerRef.APIVersion && ref.Kind == forOwnerRef.Kind) {
			// this is a specific watch
			forRef := &corev1.ObjectReference{APIVersion: r.cfg.For.APIVersion, Kind: r.cfg.For.Kind, Name: ref.Name}

			if err := r.inv.set(gvkKindCtx, []corev1.ObjectReference{*forRef, *ref}, x, false); err != nil {
				fn.Logf("error setting exisiting resource to the inventory: %v\n", err.Error())
				r.rl.AddResult(err, o)
			}
		} else {
			// this is a global watch
			if err := r.inv.set(gvkKindCtx, []corev1.ObjectReference{*ref}, x, false); err != nil {
				fn.Logf("error setting exisiting resource to the inventory: %v\n", err.Error())
				r.rl.AddResult(err, o)
			}
		}
	}
}

func (r *sdk) populateChildren() {
	// validate if the general watches are available to populate the ownResources
	if !r.inv.isReady() {
		return
	}
	// call the watch callbacks to provide info to the fns in a genric way they dont have to parse
	// to understand which resource we are trying to associate
	ready := true
	for _, resCtx := range r.inv.get(watchGVKKind, nil) {
		fn.Logf("run watch: %v\n", resCtx.existingResource)
		if resCtx.gvkKindCtx.callbackFn != nil {
			if err := resCtx.gvkKindCtx.callbackFn(resCtx.existingResource); err != nil {
				fn.Log("populatechildren not ready: watch callback failed: %v\n", err.Error())
				r.rl.AddResult(err, resCtx.existingResource)
				ready = false
			}
		}
	}
	fn.Log("populate children: ready:", ready)
	if ready {
		for forRef, resCtx := range r.inv.get(forGVKKind, nil) {
			forObj := resCtx.existingResource
			fn.Log("PopulateOwnResourcesFn", forObj)
			if r.cfg.PopulateOwnResourcesFn != nil && forObj != nil {
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
						// add the resource to the existing list as a new resource
						r.inv.set(kc, []corev1.ObjectReference{forRef, objRef}, newObj, true)
					}
				}
			}
		}
	}

	// DEBUG
	for _, entry := range r.inv.list() {
		fn.Logf("resources entry: %v\n", entry)
	}
}

// performs the update on the children after the diff in the stage1 of the pipeline
func (r *sdk) updateChildren() {
	// perform a diff to validate the existing resource against the new resources
	diff, err := r.inv.diff()
	if err != nil {
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	fn.Logf("diff: %v\n", diff)

	// if the fn is not ready to act we stop immediately
	if !r.inv.isReady() {
		// delete all child resources by setting the annotation and set the condition to false
		for _, obj := range diff.deleteObjs {
			fn.Logf("delete set condition: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			r.handleUpdate(actionDelete, obj, "not ready", true)
		}
	} else {
		// act upon the diff
		// update conditions
		for _, obj := range diff.createConditions {
			fn.Logf("create condition: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			r.setConditionInKptFile(actionCreate, obj, "condition again as it was deleted")
		}
		for _, obj := range diff.deleteConditions {
			fn.Logf("delete condition: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			r.deleteConditionInKptFile(obj)
		}
		// update resources
		for _, obj := range diff.createObjs {
			fn.Logf("create obj: ref: %s, ownkind: %s\n", kptfilelibv1.GetConditionType(&obj.ref), obj.ownKind)
			r.handleUpdate(actionCreate, obj, "resource", false)
		}
		for _, obj := range diff.updateObjs {
			fn.Logf("update obj: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			r.handleUpdate(actionUpdate, obj, "resource", false)
		}
		for _, obj := range diff.deleteObjs {
			fn.Logf("delete obj: %s\n", kptfilelibv1.GetConditionType(&obj.ref))
			// create condition - add resource to resource list
			r.handleUpdate(actionDelete, obj, "resource", true)
		}
		// this is a corner case, in case for object gets deleted and recreated
		// if the delete annotation is set, we need to cleanup the
		// delete annotation and set the condition to update
		for _, obj := range diff.updateDeleteAnnotations {
			fn.Log("update delete annotation")
			r.handleUpdate(actionCreate, obj, "resource", true)
		}

	}
	// update the kptfile with the latest consitions
	kptfile, err := r.kptf.ParseKubeObject()
	if err != nil {
		fn.Log(err)
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	r.rl.SetObject(kptfile)
}

// handleUpdate sets the condition and resource based on the action
func (r *sdk) handleUpdate(a action, obj *object, msg string, ignoreOwnKind bool) {
	// set the condition
	r.setConditionInKptFile(a, obj, msg)
	// update resource
	if a == actionDelete {
		obj.obj.SetAnnotation(FnRuntimeDelete, "true")
	}
	// set resource
	if ignoreOwnKind {
		r.setObjectInResourceList(obj)
	} else {
		if obj.ownKind == ResourceKindFull {
			r.setObjectInResourceList(obj)
		}
	}
}

func (r *sdk) deleteConditionInKptFile(obj *object) {
	// delete condition
	r.kptf.DeleteCondition(kptfilelibv1.GetConditionType(&obj.ref))
	// update the status back in the inventory
	r.inv.delete(&gvkKindCtx{gvkKind: ownGVKKind}, []corev1.ObjectReference{obj.forRef, obj.ref})
}

func (r *sdk) setConditionInKptFile(a action, obj *object, msg string) {
	c := kptv1.Condition{
		Type:    kptfilelibv1.GetConditionType(&obj.ref),
		Status:  kptv1.ConditionFalse,
		Reason:  fmt.Sprintf("%s.%s", kptfilelibv1.GetConditionType(&r.cfg.For), obj.ref.Name),
		Message: fmt.Sprintf("%s %s", a, msg),
	}
	r.kptf.SetConditions(c)
	// update the condition status back in the inventory
	r.inv.set(&gvkKindCtx{gvkKind: ownGVKKind}, []corev1.ObjectReference{obj.forRef, obj.ref}, &c, false)
}

func (r *sdk) setObjectInResourceList(obj *object) {
	r.rl.SetObject(&obj.obj)
	// update the resource status back in the inventory
	r.inv.set(&gvkKindCtx{gvkKind: ownGVKKind}, []corev1.ObjectReference{obj.forRef, obj.ref}, &obj.obj, false)
}

// updateResource updates the resource and when complete sets the condition
// to true
func (r *sdk) generateResource() {
	// get the kpt file
	kf, err := kptfilelibv1.New(r.rl.GetObjects()[0].String())
	if err != nil {
		fn.Log("error parsing kptfile")
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}

	//watches, ready := r.inv.isReady()
	if !r.inv.isReady() {
		// when the overal status is not ready delete all resources
		// TBD if we need to check the delete annotation
		readyMap := r.inv.getReadyMap()
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
	//ready := true
	for _, resCtx := range r.inv.get(watchGVKKind, nil) {
		if resCtx.gvkKindCtx.callbackFn != nil {
			if err := resCtx.gvkKindCtx.callbackFn(resCtx.existingResource); err != nil {
				fn.Log("populatechildren not ready: watch callback failed: %v", err.Error())
				r.rl.AddResult(err, resCtx.existingResource)
				//ready = false
			}
			return
		}
	}
	// the overall status is ready, so lets check the readiness map
	readyMap := r.inv.getReadyMap()
	for forRef, readyCtx := range readyMap {
		// if the for is not ready delete the object
		if !readyCtx.Ready {
			if readyCtx.ForObj != nil {
				// TBD if this is the right approach -> avoids deleting interface
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
