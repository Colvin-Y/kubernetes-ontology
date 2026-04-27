package owner

import (
	"sort"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

type WorkloadTarget struct {
	ID         model.CanonicalID
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	UID        string
}

type ChainResolver struct {
	cluster string
	byUID   map[string]ownerObject
	byRef   map[string]ownerObject
}

type ownerObject struct {
	target          WorkloadTarget
	apiVersion      string
	kind            string
	namespace       string
	name            string
	uid             string
	ownerReferences []resources.OwnerReference
	workload        bool
}

func NewChainResolver(cluster string, workloads []resources.Workload, replicaSets []resources.ReplicaSet) *ChainResolver {
	resolver := &ChainResolver{
		cluster: cluster,
		byUID:   make(map[string]ownerObject, len(workloads)+len(replicaSets)),
		byRef:   make(map[string]ownerObject, len(workloads)+len(replicaSets)),
	}
	for _, workload := range workloads {
		target := WorkloadTarget{
			ID:         model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID),
			APIVersion: workload.APIVersion,
			Kind:       workload.ControllerKind,
			Namespace:  workload.Metadata.Namespace,
			Name:       workload.Metadata.Name,
			UID:        workload.Metadata.UID,
		}
		resolver.add(ownerObject{
			target:          target,
			apiVersion:      workload.APIVersion,
			kind:            workload.ControllerKind,
			namespace:       workload.Metadata.Namespace,
			name:            workload.Metadata.Name,
			uid:             workload.Metadata.UID,
			ownerReferences: workload.OwnerReferences,
			workload:        true,
		})
	}
	for _, replicaSet := range replicaSets {
		resolver.add(ownerObject{
			kind:            "ReplicaSet",
			namespace:       replicaSet.Metadata.Namespace,
			name:            replicaSet.Metadata.Name,
			uid:             replicaSet.Metadata.UID,
			ownerReferences: replicaSet.OwnerReferences,
		})
	}
	return resolver
}

func (r *ChainResolver) ResolvePodWorkloads(pod resources.Pod) []WorkloadTarget {
	if r == nil {
		return nil
	}
	return r.ResolveOwnerReferences(pod.Metadata.Namespace, pod.OwnerReferences)
}

func (r *ChainResolver) ResolveOwnerReferences(namespace string, refs []resources.OwnerReference) []WorkloadTarget {
	if r == nil {
		return nil
	}
	targets := make([]WorkloadTarget, 0)
	seenTargets := make(map[model.CanonicalID]struct{})
	for _, ref := range controllingReferences(refs) {
		for _, target := range r.resolveReference(namespace, ref, map[string]struct{}{}) {
			if _, ok := seenTargets[target.ID]; ok {
				continue
			}
			seenTargets[target.ID] = struct{}{}
			targets = append(targets, target)
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].Namespace != targets[j].Namespace {
			return targets[i].Namespace < targets[j].Namespace
		}
		if targets[i].Kind != targets[j].Kind {
			return targets[i].Kind < targets[j].Kind
		}
		if targets[i].Name != targets[j].Name {
			return targets[i].Name < targets[j].Name
		}
		return targets[i].UID < targets[j].UID
	})
	return targets
}

func (r *ChainResolver) ResolveWorkloadOwners(workload resources.Workload) []WorkloadTarget {
	if r == nil {
		return nil
	}
	return r.ResolveOwnerReferences(workload.Metadata.Namespace, workload.OwnerReferences)
}

func ResolvePodWorkloads(cluster string, workloads []resources.Workload, replicaSets []resources.ReplicaSet, pod resources.Pod) []model.CanonicalID {
	targets := NewChainResolver(cluster, workloads, replicaSets).ResolvePodWorkloads(pod)
	out := make([]model.CanonicalID, 0, len(targets))
	for _, target := range targets {
		out = append(out, target.ID)
	}
	return out
}

func (r *ChainResolver) add(obj ownerObject) {
	r.byRef[objectRefKey(obj.namespace, obj.kind, obj.name)] = obj
	if obj.apiVersion != "" {
		r.byRef[objectVersionRefKey(obj.namespace, obj.apiVersion, obj.kind, obj.name)] = obj
	}
	if obj.uid != "" {
		r.byUID[objectUIDKey(obj.namespace, obj.kind, obj.uid)] = obj
	}
}

func (r *ChainResolver) resolveReference(namespace string, ref resources.OwnerReference, seen map[string]struct{}) []WorkloadTarget {
	obj, ok := r.lookup(namespace, ref)
	if !ok {
		return nil
	}
	return r.resolveObject(obj, seen)
}

func (r *ChainResolver) resolveObject(obj ownerObject, seen map[string]struct{}) []WorkloadTarget {
	key := objectRefKey(obj.namespace, obj.kind, obj.name)
	if obj.uid != "" {
		key = objectUIDKey(obj.namespace, obj.kind, obj.uid)
	}
	if _, ok := seen[key]; ok {
		return nil
	}
	seen[key] = struct{}{}

	targets := make([]WorkloadTarget, 0, 1)
	if obj.workload {
		targets = append(targets, obj.target)
	}
	for _, ref := range controllingReferences(obj.ownerReferences) {
		targets = append(targets, r.resolveReference(obj.namespace, ref, seen)...)
	}
	return targets
}

func (r *ChainResolver) lookup(namespace string, ref resources.OwnerReference) (ownerObject, bool) {
	if ref.UID != "" {
		if obj, ok := r.byUID[objectUIDKey(namespace, ref.Kind, ref.UID)]; ok {
			return obj, true
		}
	}
	if ref.APIVersion != "" {
		if obj, ok := r.byRef[objectVersionRefKey(namespace, ref.APIVersion, ref.Kind, ref.Name)]; ok {
			return obj, true
		}
	}
	obj, ok := r.byRef[objectRefKey(namespace, ref.Kind, ref.Name)]
	return obj, ok
}

func controllingReferences(refs []resources.OwnerReference) []resources.OwnerReference {
	controllers := make([]resources.OwnerReference, 0, 1)
	for _, ref := range refs {
		if ref.Controller {
			controllers = append(controllers, ref)
		}
	}
	if len(controllers) > 0 {
		return controllers
	}
	return refs
}

func objectRefKey(namespace, kind, name string) string {
	return namespace + "/" + kind + "/" + name
}

func objectVersionRefKey(namespace, apiVersion, kind, name string) string {
	return namespace + "/" + apiVersion + "/" + kind + "/" + name
}

func objectUIDKey(namespace, kind, uid string) string {
	return namespace + "/" + kind + "/" + uid
}
