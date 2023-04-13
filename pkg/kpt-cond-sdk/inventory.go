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
	"sync"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

type Inventory interface {
	// internal
	initializeGVKInventory(cfg *KptCondSDKConfig) error
	addGVKObjectReference(kc *kindCtx, ref corev1.ObjectReference) error
	// used through interface
	isGVKMatch(ref *corev1.ObjectReference) (*kindCtx, bool)
	setExistingCondition(*kindCtx, *corev1.ObjectReference, *corev1.ObjectReference, *kptv1.Condition) error
	deleteExistingCondition(*kindCtx, *corev1.ObjectReference, *corev1.ObjectReference) error
	setExistingResource(*kindCtx, *corev1.ObjectReference, *corev1.ObjectReference, *fn.KubeObject) error
	setNewResource(*kindCtx, *corev1.ObjectReference, *corev1.ObjectReference, *fn.KubeObject) error

	isReady() ([]*watchCtx, bool)
	getForResources() map[corev1.ObjectReference]*fn.KubeObject
	getResourceReadyMap() map[corev1.ObjectReference]*readyCtx
	diff() (inventoryDiff, error)
}

func newInventory(cfg *KptCondSDKConfig) (Inventory, error) {
	r := &inventory{
		gvkResources:   map[corev1.ObjectReference]*kindCtx{},
		forResources:   map[corev1.ObjectReference]*resourceInventory{},
		watchResources: map[corev1.ObjectReference]*resourceCtx{},
	}
	if err := r.initializeGVKInventory(cfg); err != nil {
		return nil, err
	}
	return r, nil
}

type inventory struct {
	m      sync.RWMutex
	hasOwn bool
	// gvkResource contain the gvk based resource from config
	// they dont contain the names but allow for faster lookups
	// when walking the resource list or condition list
	gvkResources map[corev1.ObjectReference]*kindCtx
	// the following maps contains the real inventory
	forResources   map[corev1.ObjectReference]*resourceInventory // specific for map with all its dependent resources own/watch
	watchResources map[corev1.ObjectReference]*resourceCtx       // general watches that dont belong to a specific for resource
}

type kind string

const (
	forkind   kind = "for"
	ownkind   kind = "own"
	watchkind kind = "watch"
)

type kindCtx struct {
	kind       kind
	ownKind    OwnKind         // only used for kind == own
	callbackFn WatchCallbackFn // only used for watches == global
}

type resourceInventory struct {
	resourceCtx
	ownResources   map[corev1.ObjectReference]*resourceCtx // those own resources are a result of a specific for
	watchResources map[corev1.ObjectReference]*resourceCtx // these watch resources have a relationship with the for - we treat them together
}

type resourceCtx struct {
	kindCtx
	existingCondition *kptv1.Condition // contains owner in the condition reason
	existingResource  *fn.KubeObject   // contains the owner in the owner annotation
	newResource       *fn.KubeObject
}

func (r *inventory) initializeGVKInventory(cfg *KptCondSDKConfig) error {
	if err := r.addGVKObjectReference(&kindCtx{kind: forkind}, cfg.For); err != nil {
		return err
	}
	for ref, ok := range cfg.Owns {
		r.hasOwn = true
		if err := r.addGVKObjectReference(&kindCtx{kind: ownkind, ownKind: ok}, ref); err != nil {
			return err
		}
	}
	for ref, cb := range cfg.Watch {
		r.hasOwn = true
		if err := r.addGVKObjectReference(&kindCtx{kind: watchkind, callbackFn: cb}, ref); err != nil {
			return err
		}
	}
	return nil
}

func (r *inventory) addGVKObjectReference(kc *kindCtx, ref corev1.ObjectReference) error {
	r.m.Lock()
	defer r.m.Unlock()

	// validates if we GVK(s) were added to the same context
	if resCtx, ok := r.gvkResources[corev1.ObjectReference{APIVersion: ref.APIVersion, Kind: ref.Kind}]; ok {
		return fmt.Errorf("another resource with a different kind %s already exists", resCtx.kind)
	}
	r.gvkResources[corev1.ObjectReference{APIVersion: ref.APIVersion, Kind: ref.Kind}] = kc
	return nil
}

func (r *inventory) isGVKMatch(ref *corev1.ObjectReference) (*kindCtx, bool) {
	r.m.RLock()
	defer r.m.RUnlock()
	if ref == nil {
		return nil, false
	}
	kindCtx, ok := r.gvkResources[corev1.ObjectReference{APIVersion: ref.APIVersion, Kind: ref.Kind}]
	if !ok {
		return nil, false
	}
	return kindCtx, true
}

func (r *inventory) initForResource(rootRef *corev1.ObjectReference) {
	if _, ok := r.forResources[*rootRef]; !ok {
		fn.Log("initForResource")
		r.forResources[*rootRef] = &resourceInventory{
			ownResources:   map[corev1.ObjectReference]*resourceCtx{},
			watchResources: map[corev1.ObjectReference]*resourceCtx{},
		}
	}
}

func (r *inventory) initWatchResource(rootRef *corev1.ObjectReference) {
	if _, ok := r.watchResources[*rootRef]; !ok {
		r.watchResources[*rootRef] = &resourceCtx{}
	}
}

func (r *inventory) initSpecificWatchResource(rootRef *corev1.ObjectReference, childRef *corev1.ObjectReference) {
	r.initForResource(rootRef)
	if _, ok := r.forResources[*rootRef].watchResources[*childRef]; !ok {
		r.forResources[*rootRef].watchResources[*childRef] = &resourceCtx{}
	}
}

func (r *inventory) initSpecificOwnResource(rootRef *corev1.ObjectReference, childRef *corev1.ObjectReference) {
	r.initForResource(rootRef)
	if _, ok := r.forResources[*rootRef].ownResources[*childRef]; !ok {
		r.forResources[*rootRef].ownResources[*childRef] = &resourceCtx{}
	}
}

func (r *inventory) setExistingCondition(kc *kindCtx, rootRef *corev1.ObjectReference, childRef *corev1.ObjectReference, c *kptv1.Condition) error {
	r.m.Lock()
	defer r.m.Unlock()

	fn.Logf("setExistingCondition: kc: %v, rootRef: %v, condition: %v\n", kc, rootRef, c)

	if rootRef == nil {
		// TBD if we need to return an error
		return fmt.Errorf("setExistingCondition, rootref cannot be nil")
	}

	switch kc.kind {
	case forkind:
		r.initForResource(rootRef)
		r.forResources[*rootRef].existingCondition = c
		r.forResources[*rootRef].kindCtx = *kc
	case watchkind:
		if childRef == nil {
			// this is a global watch
			r.initWatchResource(rootRef)
			r.watchResources[*rootRef].existingCondition = c
			r.watchResources[*rootRef].kindCtx = *kc
		} else {
			// this is a specific watch belonging to a parent
			r.initSpecificWatchResource(rootRef, childRef)
			r.forResources[*rootRef].watchResources[*childRef].existingCondition = c
			r.forResources[*rootRef].watchResources[*childRef].kindCtx = *kc
		}
	case ownkind:
		if childRef == nil {
			return fmt.Errorf("setExistingCondition, childref cannot be nil for own kind")
		}
		r.initSpecificOwnResource(rootRef, childRef)
		r.forResources[*rootRef].ownResources[*childRef].existingCondition = c
		r.forResources[*rootRef].ownResources[*childRef].kindCtx = *kc
	}
	return nil
}
func (r *inventory) deleteExistingCondition(kc *kindCtx, rootRef *corev1.ObjectReference, childRef *corev1.ObjectReference) error {
	r.m.Lock()
	defer r.m.Unlock()

	if rootRef == nil {
		// TBD if we need to return an error
		return fmt.Errorf("setExistingCondition, rootref cannot be nil")
	}

	switch kc.kind {
	case forkind:
		r.initForResource(rootRef)
		r.forResources[*rootRef].existingCondition = nil
	case watchkind:
		if childRef == nil {
			r.initWatchResource(rootRef)
			r.watchResources[*rootRef].existingCondition = nil
		} else {
			r.initSpecificWatchResource(rootRef, childRef)
			r.forResources[*rootRef].watchResources[*childRef].existingCondition = nil
		}
	case ownkind:
		if childRef == nil {
			return fmt.Errorf("setExistingCondition, childref cannot be nil for own kind")
		}
		r.initSpecificOwnResource(rootRef, childRef)
		r.forResources[*rootRef].ownResources[*childRef].existingCondition = nil
	}
	return nil
}
func (r *inventory) setExistingResource(kc *kindCtx, rootRef *corev1.ObjectReference, childRef *corev1.ObjectReference, o *fn.KubeObject) error {
	r.m.Lock()
	defer r.m.Unlock()

	switch kc.kind {
	case forkind:
		fn.Logf("setExistingResource: kc: %v, rootRef: %v, o: %v\n", kc, rootRef, o)
		r.initForResource(rootRef)
		r.forResources[*rootRef].existingResource = o
		r.forResources[*rootRef].kindCtx = *kc
	case watchkind:
		if childRef == nil {
			r.initWatchResource(rootRef)
			r.watchResources[*rootRef].existingResource = o
			r.watchResources[*rootRef].kindCtx = *kc
		} else {
			r.initSpecificWatchResource(rootRef, childRef)
			r.forResources[*rootRef].watchResources[*childRef].existingResource = o
			r.forResources[*rootRef].watchResources[*childRef].kindCtx = *kc
		}
	case ownkind:
		if childRef == nil {
			return fmt.Errorf("setExistingCondition, childref cannot be nil for own kind")
		}
		r.initSpecificOwnResource(rootRef, childRef)
		r.forResources[*rootRef].ownResources[*childRef].existingResource = o
		r.forResources[*rootRef].ownResources[*childRef].kindCtx = *kc
	}
	return nil
}
func (r *inventory) setNewResource(kc *kindCtx, rootRef *corev1.ObjectReference, childRef *corev1.ObjectReference, o *fn.KubeObject) error {
	r.m.Lock()
	defer r.m.Unlock()

	fn.Logf("setNewResource: kc: %v, rootRef: %v, childRef: %v o: %v\n", kc, rootRef, childRef, o)

	switch kc.kind {
	case forkind:
		r.initForResource(rootRef)
		r.forResources[*rootRef].newResource = o
		r.forResources[*rootRef].kindCtx = *kc
	case watchkind:
		if childRef == nil {
			r.initWatchResource(rootRef)
			r.watchResources[*rootRef].newResource = o
			r.watchResources[*rootRef].kindCtx = *kc
		} else {
			r.initSpecificWatchResource(rootRef, childRef)
			r.forResources[*rootRef].watchResources[*childRef].newResource = o
			r.forResources[*rootRef].watchResources[*childRef].kindCtx = *kc
		}
	case ownkind:
		if childRef == nil {
			return fmt.Errorf("setExistingCondition, childref cannot be nil for own kind")
		}
		r.initSpecificOwnResource(rootRef, childRef)
		r.forResources[*rootRef].ownResources[*childRef].newResource = o
		r.forResources[*rootRef].ownResources[*childRef].kindCtx = *kc
	}
	return nil
}

func (r *inventory) isReady() ([]*watchCtx, bool) {
	r.m.RLock()
	defer r.m.RUnlock()
	// if no owners are needed there is no need to generate children
	// we return as not ready
	if !r.hasOwn {
		return nil, false
	}

	// check readiness, we start positive
	ready := true
	watches := make([]*watchCtx, 0, len(r.watchResources))
	// the readiness is determined by the global watch resources
	for _, resCtx := range r.watchResources {
		// if watched resource does not exist we fail readiness
		// if the condition is present and the status is False something is pending, so we
		// fail readiness
		if resCtx.existingResource == nil ||
			(resCtx.existingCondition != nil &&
				resCtx.existingCondition.Status == kptv1.ConditionStatus(corev1.ConditionFalse)) {
			ready = false
			break
		}
		watches = append(watches, &watchCtx{
			o:        resCtx.existingResource,
			callback: resCtx.callbackFn,
		})
	}
	return watches, ready
}

type watchCtx struct {
	o        *fn.KubeObject
	callback WatchCallbackFn
}

func (r *inventory) getResourceReadyMap() map[corev1.ObjectReference]*readyCtx {
	r.m.RLock()
	defer r.m.RUnlock()

	readyMap := map[corev1.ObjectReference]*readyCtx{}
	for forRef, inv := range r.forResources {
		readyMap[forRef] = &readyCtx{
			Ready:   true,
			Owns:    map[corev1.ObjectReference]fn.KubeObject{},
			Watches: map[corev1.ObjectReference]fn.KubeObject{},
			ForObj:  inv.existingResource,
		}
		for ref, resCtx := range inv.ownResources {
			if resCtx.existingCondition == nil ||
				resCtx.existingCondition.Status == kptv1.ConditionStatus(corev1.ConditionFalse) {
				readyMap[forRef].Ready = false
			}
			if resCtx.existingResource != nil {
				readyMap[forRef].Owns[ref] = *resCtx.existingResource
			}
		}
		for ref, resCtx := range inv.watchResources {
			// TODO we need to look at some watches that we want to check the condition for and others not
			if resCtx.existingCondition == nil || resCtx.existingCondition.Status == kptv1.ConditionStatus(corev1.ConditionFalse) {
				readyMap[forRef].Ready = false
			}
			if _, ok := readyMap[forRef].Watches[ref]; !ok {
				readyMap[forRef].Watches[ref] = *resCtx.existingResource
			}
			if resCtx.existingResource != nil {
				readyMap[forRef].Watches[ref] = *resCtx.existingResource
			}
		}
	}
	return readyMap
}

func (r *inventory) getForResources() map[corev1.ObjectReference]*fn.KubeObject {
	r.m.RLock()
	defer r.m.RUnlock()

	forObjs := map[corev1.ObjectReference]*fn.KubeObject{}
	for ref, inv := range r.forResources {
		fn.Log("getForResource", ref, inv.existingResource)
		if inv.existingResource != nil {
			forObjs[ref] = inv.existingResource
		}
	}
	return forObjs
}

type readyCtx struct {
	Ready   bool
	Owns    map[corev1.ObjectReference]fn.KubeObject
	Watches map[corev1.ObjectReference]fn.KubeObject
	ForObj  *fn.KubeObject
}

type inventoryDiff struct {
	deleteObjs       []*object
	updateObjs       []*object
	createObjs       []*object
	deleteConditions []*object
	createConditions []*object
	//updateConditions []*object
}

type object struct {
	forRef  corev1.ObjectReference
	ref     corev1.ObjectReference
	obj     fn.KubeObject
	ownKind OwnKind
}

// Diff is based on the following principle: we have an inventory
// populated with the existing resource/condition info and we also
// have information on new resource/condition that would be created
// if nothinf existed.
// the diff compares these the eixisiting resource/condition inventory
// agsinst the new resource/condition inventory and provide CRUD operation
// based on the comparisons.
func (r *inventory) diff() (inventoryDiff, error) {
	r.m.RLock()
	defer r.m.RUnlock()
	diff := inventoryDiff{
		deleteObjs:       []*object{},
		updateObjs:       []*object{},
		createObjs:       []*object{},
		deleteConditions: []*object{},
		createConditions: []*object{},
	}

	for forRef, inv := range r.forResources {
		// if the existing for resource is not present we need to cleanup
		// all child resources and conditions
		fn.Logf("diff: forRef: %v, existingResource: %v\n", forRef, inv.existingResource)
		if inv.existingResource == nil {
			for ref, resCtx := range inv.ownResources {
				fn.Logf("delete resource and conditions: forRef: %v, ownRef: %v\n", forRef, ref)
				if resCtx.existingCondition != nil {
					diff.deleteConditions = append(diff.deleteConditions, &object{forRef: forRef, ref: ref, ownKind: resCtx.ownKind})
				}
				if resCtx.existingResource != nil {
					diff.deleteObjs = append(diff.deleteObjs, &object{forRef: forRef, ref: ref, obj: *resCtx.existingResource, ownKind: resCtx.ownKind})
				}
			}
		} else {
			for ref, resCtx := range inv.ownResources {
				// condition diff handling
				switch {
				// if there is no new resource, but we have a condition for that resource we should delete the condition
				case resCtx.newResource == nil && resCtx.existingCondition != nil:
					diff.deleteConditions = append(diff.deleteConditions, &object{forRef: forRef, ref: ref, ownKind: resCtx.ownKind})
				// if there is a new resource, but we have no condition for that resource someone deleted it
				// and we have to recreate that condition
				case resCtx.newResource != nil && resCtx.existingCondition == nil:
					diff.createConditions = append(diff.createConditions, &object{forRef: forRef, ref: ref, obj: *resCtx.newResource, ownKind: resCtx.ownKind})
				}

				// resource diff handling
				switch {
				// if the existing resource does not exist but the new resource exist we have to create the new resource
				case resCtx.existingResource == nil && resCtx.newResource != nil:
					// create resource
					diff.createObjs = append(diff.createObjs, &object{forRef: forRef, ref: ref, obj: *resCtx.newResource, ownKind: resCtx.ownKind})
				// if the new resource does not exist and but the resource exist we have to delete the exisiting resource
				case resCtx.existingResource != nil && resCtx.newResource == nil:
					// delete resource
					diff.deleteObjs = append(diff.deleteObjs, &object{forRef: forRef, ref: ref, ownKind: resCtx.ownKind})
				// if both exisiting/new resource exists check the differences of the spec
				// dependening on the outcome update the resource with the new information
				case resCtx.existingResource != nil && resCtx.newResource != nil:
					// check diff
					existingSpec, ok, err := resCtx.existingResource.NestedStringMap("spec")
					if err != nil {
						fn.Logf("cannot get spec from for existing obj: %s, err: %v\n", ref, err.Error())
						//return inventoryDiff{}, err
					}
					if !ok {
						fn.Logf("cannot get spec from for existing obj: %s, err: %v\n", ref)
						//return inventoryDiff{}, fmt.Errorf("cannot get spec for existing object: %v", ref)
					}
					newSpec, ok, err := resCtx.newResource.NestedStringMap("spec")
					if err != nil {
						fn.Logf("cannot get spec from for new obj: %s, err: %v\n", ref, err.Error())
						//return inventoryDiff{}, err
					}
					if !ok {
						fn.Logf("cannot get spec from for new obj: %s, err: %v\n", ref)
						//return inventoryDiff{}, fmt.Errorf("cannot get spec of new object: %v", ref)
					}

					if d := cmp.Diff(existingSpec, newSpec); d != "" {
						diff.updateObjs = append(diff.updateObjs, &object{forRef: forRef, ref: ref, obj: *resCtx.newResource, ownKind: resCtx.ownKind})
					}
				}
			}
		}
	}
	return diff, nil
}
