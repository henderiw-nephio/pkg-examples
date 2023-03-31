package fnruntime

import (
	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	corev1 "k8s.io/api/core/v1"
)

type DownstreamInventory interface {
	AddCondition(owner corev1.ObjectReference, resource corev1.ObjectReference, c *kptv1.Condition)
	AddResource(owner corev1.ObjectReference, resource corev1.ObjectReference, o *fn.KubeObject)
	AddForCondition(owner corev1.ObjectReference, c *kptv1.Condition)
	AddForResource(owner corev1.ObjectReference, o *fn.KubeObject)
	GetForConditionStatus(owner corev1.ObjectReference) bool
	IsReady() map[corev1.ObjectReference]*ReadyCtx
}

func NewDownstreamInventory() DownstreamInventory {
	return &downstreamInventory{
		ownerResources: map[corev1.ObjectReference]*downstreamResources{},
	}
}

type downstreamInventory struct {
	ownerResources map[corev1.ObjectReference]*downstreamResources
}

type downstreamResources struct {
	resources    map[corev1.ObjectReference]*downstreamInventoryCtx
	forObj       *fn.KubeObject
	forCondition *kptv1.Condition
}

type downstreamInventoryCtx struct {
	condition *kptv1.Condition
	obj       *fn.KubeObject
}

func (r *downstreamInventory) AddCondition(owner corev1.ObjectReference, resource corev1.ObjectReference, c *kptv1.Condition) {
	if _, ok := r.ownerResources[owner]; !ok {
		r.ownerResources[owner] = &downstreamResources{
			resources: map[corev1.ObjectReference]*downstreamInventoryCtx{},
		}
	}
	if r.ownerResources[owner].resources[resource] == nil {
		r.ownerResources[owner].resources[resource] = &downstreamInventoryCtx{}
	}
	r.ownerResources[owner].resources[resource].condition = c
}

func (r *downstreamInventory) AddResource(owner corev1.ObjectReference, resource corev1.ObjectReference, o *fn.KubeObject) {
	if _, ok := r.ownerResources[owner]; !ok {
		r.ownerResources[owner] = &downstreamResources{
			resources: map[corev1.ObjectReference]*downstreamInventoryCtx{},
		}
	}
	if r.ownerResources[owner].resources[resource] == nil {
		r.ownerResources[owner].resources[resource] = &downstreamInventoryCtx{}
	}
	r.ownerResources[owner].resources[resource].obj = o
}

func (r *downstreamInventory) AddForCondition(owner corev1.ObjectReference, c *kptv1.Condition) {
	if _, ok := r.ownerResources[owner]; !ok {
		r.ownerResources[owner] = &downstreamResources{
			resources: map[corev1.ObjectReference]*downstreamInventoryCtx{},
		}
	}
	r.ownerResources[owner].forCondition = c
}

func (r *downstreamInventory) AddForResource(owner corev1.ObjectReference, o *fn.KubeObject) {
	if _, ok := r.ownerResources[owner]; !ok {
		r.ownerResources[owner] = &downstreamResources{
			resources: map[corev1.ObjectReference]*downstreamInventoryCtx{},
		}
	}
	r.ownerResources[owner].forObj = o
}

func (r *downstreamInventory) GetForConditionStatus(owner corev1.ObjectReference) bool {
	c, ok := r.ownerResources[owner]
	if !ok {
		// this should not happen
		return false
	}
	return c.forCondition.Status == kptv1.ConditionFalse

}

func (r *downstreamInventory) IsReady() map[corev1.ObjectReference]*ReadyCtx {
	readyMap := map[corev1.ObjectReference]*ReadyCtx{}
	for ownerRef, ref := range r.ownerResources {
		readyMap[ownerRef] = &ReadyCtx{
			ForObj:       ref.forObj,
			ForCondition: ref.forCondition,
		}
		for objRef, invCtx := range ref.resources {
			// this is a strange case as the condition is ready but there is no related resource
			// to be flagged
			if invCtx.obj == nil || invCtx.condition.Status == kptv1.ConditionFalse {
				readyMap[ownerRef].Ready = false
				break
			}
			readyMap[ownerRef].Objs[objRef] = invCtx.obj
		}
		readyMap[ownerRef].Ready = true
	}
	return readyMap
}

type ReadyCtx struct {
	Ready        bool
	Objs         map[corev1.ObjectReference]*fn.KubeObject
	ForObj       *fn.KubeObject
	ForCondition *kptv1.Condition
}
