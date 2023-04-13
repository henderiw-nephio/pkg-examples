# fn runtime

event driven -> porch scheduled
ownerRef -> condition.Reason and/or annotation with OwnerKey
deleteTstp -> condition.Status = False and/or annotation with DeleteKey
finalizer -> we never delete directly but wait for the downstream fn to perform the delete
for - for fn (upstream/downstream fn)
owns - upstream fn: child resource that get created; downstream fn the resources it depends upon
watch - extra resources relevant for this function

condition: signals if work is required

upstream fn:
- provides lifecycle on child resources (owns) based on a parent resource (for)
- adjacent resources are used (watch) to decide and influence the upstream behavior
- types of child resources (owns):
    - downstream child resurces:
        - condition: signals to the downstream function to perform work (used for all CRUD operation)
        - create/update: performed by downstream fn (not all data is available upstream)
        - delete: performed downstream
    - upstream child resources:
        - condition: signals to the downstream function to perform work (used for all CRUD operation)
        - create/update: performed by upstream fn (all data available upstream)
        - delete: performed downstream

downstream fn
- provides lifecycle on a resource (for)
    - delete: 
        - if resource exists and some conditions are not ready
        - if the delete annotation is set
    - update/create: if all adjacent conditions are met
- adjacent resources are used (watch)
- contributing resources (owns)
- types: wildcard downstream Fn (last Fn) and specific downstream Fn