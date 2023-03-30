package fnruntime

import (
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	kptv1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	"github.com/example.com/foo/pkg/inventory"
	kptfilelibv1 "github.com/example.com/foo/pkg/kptfile/v1"
	"github.com/example.com/foo/pkg/kptrl"
	corev1 "k8s.io/api/core/v1"
)

const FnRuntimeOwner = "fnruntime.nephio.org/owner"
const FnRuntimeDelete = "fnruntime.nephio.org/delete"

type FnRuntime interface {
	Run()
}

type Config struct {
	For         ForConfig
	Owns        map[corev1.ObjectReference]ConfigOperation
	Watch       map[corev1.ObjectReference]WatchCallbackFn
	ConditionFn ConditionFn
}

type ForConfig struct {
	ObjectRef  corev1.ObjectReference
	PopulateFn PopulateFn
}

type ConfigOperation string

const (
	ConfigOperationDefault       ConfigOperation = "default"
	ConfigOperationConditionOnly ConfigOperation = "conditionOnly"
)

type WatchCallbackFn func(o *fn.KubeObject) error

type PopulateFn func(o *fn.KubeObject) (map[corev1.ObjectReference]*fn.KubeObject, error)

func populateFnNop(o *fn.KubeObject) (map[corev1.ObjectReference]*fn.KubeObject, error) {
	return map[corev1.ObjectReference]*fn.KubeObject{}, nil
}

type ConditionFn func() bool

func conditionFnNop() bool {
	return true
}

func New(rl *fn.ResourceList, c *Config) FnRuntime {
	r := &fnRuntime{
		cfg:         c,
		inventory:   inventory.New(),
		rl:          kptrl.New(rl),
		conditionFn: conditionFnNop,
	}
	if r.cfg.ConditionFn != nil {
		r.conditionFn = r.cfg.ConditionFn
	}

	return r
}

type fnRuntime struct {
	cfg         *Config
	inventory   inventory.Inventory
	rl          kptrl.ResourceList
	conditionFn ConditionFn
}

func (r *fnRuntime) Run() {
	r.initialize()
	r.populate()
	r.update()
}

// initialize updates the inventory based on the interested resources
// kptfile conditions
// own and watch ressources from the config
func (r *fnRuntime) initialize() {
	for _, o := range r.rl.GetObjects() {
		if o.GetAPIVersion() == kptv1.KptFileGVK().GroupVersion().String() && o.GetKind() == kptv1.KptFileName {
			kf := kptfilelibv1.NewMutator(o.String())
			var err error
			if _, err = kf.UnMarshal(); err != nil {
				fn.Log("error unmarshal kptfile in initialize")
				r.rl.AddResult(err, o)
			}

			// populate condition inventory
			for objRef := range r.cfg.Owns {
				for _, c := range kf.GetConditions() {
					if strings.Contains(c.Type, kptfilelibv1.GetConditionType(&objRef)) &&
						c.Reason == kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef) {
						r.inventory.AddExistingCondition(kptfilelibv1.GetGVKNFromConditionType(c.Type), &c)
					}
				}
			}
		}

		for objRef := range r.cfg.Owns {
			if o.GetAPIVersion() == objRef.APIVersion && o.GetKind() == objRef.Kind &&
				o.GetAnnotation(FnRuntimeOwner) == kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef) {

				r.inventory.AddExistingResource(&corev1.ObjectReference{
					APIVersion: objRef.APIVersion,
					Kind:       objRef.Kind,
					Name:       o.GetName(),
				}, o)
			}
		}

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
}

func (r *fnRuntime) populate() {
	// func generalPopulateConditionFn() bool
	if r.conditionFn() {
		for _, o := range r.rl.GetObjects() {
			if o.GetAPIVersion() == r.cfg.For.ObjectRef.APIVersion && o.GetKind() == r.cfg.For.ObjectRef.Kind {
				if r.cfg.For.PopulateFn != nil {
					res, err := r.cfg.For.PopulateFn(o)
					if err != nil {
						r.rl.AddResult(err, o)
					} else {
						for objRef, newObj := range res {
							newObj.SetAnnotation(FnRuntimeOwner, kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef))
							r.inventory.AddNewResource(&corev1.ObjectReference{
								APIVersion: objRef.APIVersion,
								Kind:       objRef.Kind,
								Name:       o.GetName(),
							}, newObj)
						}
					}
				}
			}
		}
	}
}

func (r *fnRuntime) update() {
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

	if !r.conditionFn() {
		// set deletion timestamp on all resources
		for _, obj := range diff.DeleteObjs {
			fn.Logf("create set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// set condition
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef),
				Message: "cluster context has no site id",
			})
			// update the release timestamp
			obj.Obj.SetAnnotation(FnRuntimeDelete, "true")
			r.rl.SetObject(&obj.Obj)
		}
		return
	} else {
		for _, obj := range diff.CreateConditions {
			fn.Logf("create condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// create condition again
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef),
				Message: "create condition again as it was deleted",
			})
		}
		for _, obj := range diff.DeleteConditions {
			fn.Logf("delete condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// delete condition
			kf.DeleteCondition(kptfilelibv1.GetConditionType(&obj.Ref))
		}
		for _, obj := range diff.CreateObjs {
			fn.Logf("create set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// create condition - add resource to resource list
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef),
				Message: "create new resource",
			})

			if r.cfg.Owns[corev1.ObjectReference{APIVersion: obj.Ref.APIVersion, Kind: obj.Ref.Kind}] == ConfigOperationDefault {
				r.rl.AddObject(&obj.Obj)
			}
		}
		for _, obj := range diff.UpdateObjs {
			fn.Logf("update set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// update condition - add resource to resource list
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef),
				Message: "update existing resource",
			})
			if r.cfg.Owns[corev1.ObjectReference{APIVersion: obj.Ref.APIVersion, Kind: obj.Ref.Kind}] == ConfigOperationDefault {
				r.rl.SetObject(&obj.Obj)
			}
		}
		for _, obj := range diff.DeleteObjs {
			fn.Logf("update set condition: %s\n", kptfilelibv1.GetConditionType(&obj.Ref))
			// create condition - add resource to resource list
			kf.SetConditions(kptv1.Condition{
				Type:   kptfilelibv1.GetConditionType(&obj.Ref),
				Status: kptv1.ConditionFalse,
				Reason: kptfilelibv1.GetConditionType(&r.cfg.For.ObjectRef),
				Message: "delete existing resource",
			})
			// update resource to resoucelist with delete Timestamp set
			obj.Obj.SetAnnotation(FnRuntimeDelete, "true")
			r.rl.SetObject(&obj.Obj)
		}
	}

	kptfile, err := kf.ParseKubeObject()
	if err != nil {
		fn.Log(err)
		r.rl.AddResult(err, r.rl.GetObjects()[0])
	}
	r.rl.SetObject(kptfile)
}
