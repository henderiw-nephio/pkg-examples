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
	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	corev1 "k8s.io/api/core/v1"
)

type readyCtx struct {
	Ready   bool
	ForObj  *fn.KubeObject
	Owns    map[corev1.ObjectReference]fn.KubeObject
	Watches map[corev1.ObjectReference]fn.KubeObject
}

func (r *inventory) isReady() bool {
	r.m.RLock()
	defer r.m.RUnlock()
	// if no owners are needed there is no need to generate children
	// we return as not ready
	if !r.hasOwn {
		return false
	}

	// check readiness, we start positive
	ready := true
	// the readiness is determined by the global watch resources
	for _, resCtx := range r.get(watchGVKKind, nil) {
		// if watched resource does not exist we fail readiness
		// if the condition is present and the status is False something is pending, so we
		// fail readiness
		if resCtx.existingResource == nil ||
			(resCtx.existingCondition != nil &&
				resCtx.existingCondition.Status == kptv1.ConditionStatus(corev1.ConditionFalse)) {
			ready = false
			break
		}
	}
	return ready
}

func (r *inventory) getResourceReadyMap() map[corev1.ObjectReference]*readyCtx {
	r.m.RLock()
	defer r.m.RUnlock()

	readyMap := map[corev1.ObjectReference]*readyCtx{}
	for forRef, resCtx := range r.get(forGVKKind, nil) {
		readyMap[forRef] = &readyCtx{
			Ready:   true,
			Owns:    map[corev1.ObjectReference]fn.KubeObject{},
			Watches: map[corev1.ObjectReference]fn.KubeObject{},
			ForObj:  resCtx.existingResource,
		}
		for ref, resCtx := range r.get(ownGVKKind, &forRef) {
			if resCtx.existingCondition == nil ||
				resCtx.existingCondition.Status == kptv1.ConditionStatus(corev1.ConditionFalse) {
				readyMap[forRef].Ready = false
			}
			if resCtx.existingResource != nil {
				readyMap[forRef].Owns[ref] = *resCtx.existingResource
			}
		}
		for ref, resCtx := range r.get(watchGVKKind, &forRef) {
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
