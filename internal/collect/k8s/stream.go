package k8s

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/owner"
)

type Stream interface {
	Run(ctx context.Context, sink ChangeSink) error
}

type StreamMode string

const (
	StreamModeInformer         StreamMode = "informer"
	StreamModePolling          StreamMode = "polling"
	defaultInformerSyncTimeout            = 2 * time.Minute
)

func ParseStreamMode(raw string) (StreamMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(StreamModeInformer):
		return StreamModeInformer, nil
	case string(StreamModePolling):
		return StreamModePolling, nil
	default:
		return "", fmt.Errorf("stream mode must be %q or %q", StreamModeInformer, StreamModePolling)
	}
}

type InformerStreamOptions struct {
	ContextNamespaces []string
	DynamicClient     dynamic.Interface
	WorkloadResources []WorkloadResource
	ResyncPeriod      time.Duration
	SyncTimeout       time.Duration
}

type InformerStream struct {
	client            kubernetes.Interface
	dynamicClient     dynamic.Interface
	contextNamespaces []string
	workloadResources []WorkloadResource
	resyncPeriod      time.Duration
	syncTimeout       time.Duration
}

type WatchStream = InformerStream

func NewInformerStream(client kubernetes.Interface, options InformerStreamOptions) *InformerStream {
	return &InformerStream{
		client:            client,
		dynamicClient:     options.DynamicClient,
		contextNamespaces: append([]string(nil), options.ContextNamespaces...),
		workloadResources: append([]WorkloadResource(nil), options.WorkloadResources...),
		resyncPeriod:      options.ResyncPeriod,
		syncTimeout:       options.SyncTimeout,
	}
}

func NewWatchStream(client kubernetes.Interface, options InformerStreamOptions) *WatchStream {
	return NewInformerStream(client, options)
}

func (s *InformerStream) Run(ctx context.Context, sink ChangeSink) error {
	if s == nil || s.client == nil {
		return errors.New("informer stream requires kubernetes client")
	}
	if sink == nil {
		return errors.New("informer stream requires sink")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	events := make(chan ChangeEvent, 1024)
	handler := cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj interface{}, isInInitialList bool) {
			if isInInitialList {
				return
			}
			enqueueInformerEvent(runCtx, events, obj, ChangeTypeUpsert)
		},
		UpdateFunc: func(_, newObj interface{}) {
			enqueueInformerEvent(runCtx, events, newObj, ChangeTypeUpsert)
		},
		DeleteFunc: func(obj interface{}) {
			enqueueInformerEvent(runCtx, events, obj, ChangeTypeDelete)
		},
	}

	var starters []func(<-chan struct{})
	var syncs []cache.InformerSynced
	if err := s.registerNamespacedInformers(handler, &starters, &syncs); err != nil {
		return err
	}
	if err := s.registerClusterScopedInformers(handler, &starters, &syncs); err != nil {
		return err
	}
	if err := s.registerDynamicWorkloadInformers(handler, &starters, &syncs); err != nil {
		return err
	}

	for _, start := range starters {
		start(runCtx.Done())
	}

	syncCtx := runCtx
	var syncCancel context.CancelFunc
	if s.syncTimeout >= 0 {
		timeout := s.syncTimeout
		if timeout == 0 {
			timeout = defaultInformerSyncTimeout
		}
		syncCtx, syncCancel = context.WithTimeout(runCtx, timeout)
		defer syncCancel()
	}
	if !cache.WaitForCacheSync(syncCtx.Done(), syncs...) {
		if runCtx.Err() != nil {
			return runCtx.Err()
		}
		if syncCtx.Err() != nil {
			return fmt.Errorf("informer stream cache sync: %w", syncCtx.Err())
		}
		return errors.New("informer stream cache sync failed")
	}

	for {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case event := <-events:
			if err := sink.Apply(runCtx, event); err != nil {
				return err
			}
		}
	}
}

func (s *InformerStream) registerNamespacedInformers(handler cache.ResourceEventHandler, starters *[]func(<-chan struct{}), syncs *[]cache.InformerSynced) error {
	for _, namespace := range informerNamespaces(s.contextNamespaces) {
		factory := informers.NewSharedInformerFactoryWithOptions(s.client, s.resyncPeriod, informers.WithNamespace(namespace))
		for _, informer := range []cache.SharedIndexInformer{
			factory.Apps().V1().Deployments().Informer(),
			factory.Apps().V1().StatefulSets().Informer(),
			factory.Apps().V1().DaemonSets().Informer(),
			factory.Apps().V1().ReplicaSets().Informer(),
			factory.Batch().V1().Jobs().Informer(),
			factory.Core().V1().Pods().Informer(),
			factory.Core().V1().Services().Informer(),
			factory.Core().V1().PersistentVolumeClaims().Informer(),
			factory.Core().V1().Events().Informer(),
			factory.Core().V1().ServiceAccounts().Informer(),
			factory.Rbac().V1().RoleBindings().Informer(),
		} {
			if err := registerInformer(informer, handler, syncs); err != nil {
				return err
			}
		}
		*starters = append(*starters, factory.Start)
	}
	return nil
}

func (s *InformerStream) registerClusterScopedInformers(handler cache.ResourceEventHandler, starters *[]func(<-chan struct{}), syncs *[]cache.InformerSynced) error {
	factory := informers.NewSharedInformerFactory(s.client, s.resyncPeriod)
	for _, informer := range []cache.SharedIndexInformer{
		factory.Core().V1().PersistentVolumes().Informer(),
		factory.Storage().V1().StorageClasses().Informer(),
		factory.Storage().V1().CSIDrivers().Informer(),
		factory.Rbac().V1().ClusterRoleBindings().Informer(),
	} {
		if err := registerInformer(informer, handler, syncs); err != nil {
			return err
		}
	}
	*starters = append(*starters, factory.Start)
	return nil
}

func (s *InformerStream) registerDynamicWorkloadInformers(handler cache.ResourceEventHandler, starters *[]func(<-chan struct{}), syncs *[]cache.InformerSynced) error {
	if len(s.workloadResources) == 0 {
		return nil
	}
	if s.dynamicClient == nil {
		return errors.New("informer stream requires dynamic client for custom workload resources")
	}

	namespaced := make([]WorkloadResource, 0, len(s.workloadResources))
	clusterScoped := make([]WorkloadResource, 0, len(s.workloadResources))
	for _, resource := range s.workloadResources {
		if resource.Namespaced {
			namespaced = append(namespaced, resource)
		} else {
			clusterScoped = append(clusterScoped, resource)
		}
	}

	if len(namespaced) > 0 {
		for _, namespace := range informerNamespaces(s.contextNamespaces) {
			factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(s.dynamicClient, s.resyncPeriod, namespace, nil)
			for _, resource := range namespaced {
				if err := registerInformer(factory.ForResource(resource.GVR()).Informer(), handler, syncs); err != nil {
					return err
				}
			}
			*starters = append(*starters, factory.Start)
		}
	}
	if len(clusterScoped) > 0 {
		factory := dynamicinformer.NewDynamicSharedInformerFactory(s.dynamicClient, s.resyncPeriod)
		for _, resource := range clusterScoped {
			if err := registerInformer(factory.ForResource(resource.GVR()).Informer(), handler, syncs); err != nil {
				return err
			}
		}
		*starters = append(*starters, factory.Start)
	}
	return nil
}

func registerInformer(informer cache.SharedIndexInformer, handler cache.ResourceEventHandler, syncs *[]cache.InformerSynced) error {
	if _, err := informer.AddEventHandler(handler); err != nil {
		return err
	}
	*syncs = append(*syncs, informer.HasSynced)
	return nil
}

func enqueueInformerEvent(ctx context.Context, events chan<- ChangeEvent, obj interface{}, change ChangeType) {
	event, ok := changeEventForInformerObject(obj, change)
	if !ok {
		return
	}
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func changeEventForInformerObject(obj interface{}, change ChangeType) (ChangeEvent, bool) {
	obj = normalizeInformerObject(unwrapInformerObject(obj))
	accessor, err := apimeta.Accessor(obj)
	if err != nil {
		return ChangeEvent{}, false
	}
	kind := informerChangeKind(obj)
	if kind == "" {
		return ChangeEvent{}, false
	}
	name := accessor.GetName()
	if _, ok := obj.(*appsv1.ReplicaSet); ok {
		if ownerName := controllerOwnerName(accessor.GetOwnerReferences()); ownerName != "" {
			name = ownerName
		}
	}
	return ChangeEvent{
		Kind:      kind,
		Namespace: accessor.GetNamespace(),
		Name:      name,
		Change:    change,
	}, true
}

func informerChangeKind(obj interface{}) string {
	switch obj.(type) {
	case *appsv1.Deployment, *appsv1.StatefulSet, *appsv1.DaemonSet, *appsv1.ReplicaSet, *batchv1.Job, *unstructured.Unstructured:
		return "workload"
	case *corev1.Pod:
		return "pod"
	case *corev1.Service:
		return "service"
	case *corev1.PersistentVolumeClaim, *corev1.PersistentVolume, *storagev1.StorageClass, *storagev1.CSIDriver:
		return "storage"
	case *corev1.Event:
		return "event"
	case *corev1.ServiceAccount, *rbacv1.RoleBinding, *rbacv1.ClusterRoleBinding:
		return "identity/security"
	default:
		return ""
	}
}

func unwrapInformerObject(obj interface{}) interface{} {
	switch item := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		return item.Obj
	case *cache.DeletedFinalStateUnknown:
		if item == nil {
			return nil
		}
		return item.Obj
	default:
		return obj
	}
}

func normalizeInformerObject(obj interface{}) interface{} {
	switch item := obj.(type) {
	case appsv1.Deployment:
		return &item
	case appsv1.StatefulSet:
		return &item
	case appsv1.DaemonSet:
		return &item
	case appsv1.ReplicaSet:
		return &item
	case batchv1.Job:
		return &item
	case corev1.Pod:
		return &item
	case corev1.Service:
		return &item
	case corev1.PersistentVolumeClaim:
		return &item
	case corev1.PersistentVolume:
		return &item
	case corev1.Event:
		return &item
	case corev1.ServiceAccount:
		return &item
	case rbacv1.RoleBinding:
		return &item
	case rbacv1.ClusterRoleBinding:
		return &item
	case storagev1.StorageClass:
		return &item
	case storagev1.CSIDriver:
		return &item
	case unstructured.Unstructured:
		return &item
	default:
		return obj
	}
}

func controllerOwnerName(refs []metav1.OwnerReference) string {
	for _, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return ref.Name
		}
	}
	return ""
}

func informerNamespaces(contextNamespaces []string) []string {
	seen := make(map[string]struct{}, len(contextNamespaces))
	out := make([]string, 0, len(contextNamespaces))
	for _, namespace := range contextNamespaces {
		namespace = strings.TrimSpace(namespace)
		if namespace == "" {
			continue
		}
		if namespace == metav1.NamespaceAll {
			return []string{metav1.NamespaceAll}
		}
		if _, ok := seen[namespace]; ok {
			continue
		}
		seen[namespace] = struct{}{}
		out = append(out, namespace)
	}
	if len(out) == 0 {
		return []string{metav1.NamespaceAll}
	}
	return out
}

type PollingStream struct {
	collector          Collector
	interval           time.Duration
	lastHash           uint64
	lastCategoryHashes map[string]uint64
	lastSnapshot       Snapshot
	started            bool
}

func NewPollingStream(collector Collector, interval time.Duration) *PollingStream {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &PollingStream{collector: collector, interval: interval}
}

func (s *PollingStream) Run(ctx context.Context, sink ChangeSink) error {
	if s.collector == nil {
		return errors.New("polling stream requires collector")
	}
	if sink == nil {
		return errors.New("polling stream requires sink")
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		if err := s.tick(ctx, sink); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *PollingStream) tick(ctx context.Context, sink ChangeSink) error {
	snapshot, err := s.collector.Collect(ctx)
	if err != nil {
		return err
	}
	hash, categories := snapshotFingerprint(snapshot)
	if !s.started {
		s.started = true
		s.lastHash = hash
		s.lastCategoryHashes = categoryHashMap(categories)
		s.lastSnapshot = snapshot
		return nil
	}
	if hash == s.lastHash {
		return nil
	}
	kind := firstChangedKind(s.lastCategoryHashes, categories)
	namespace, name := changedObjectRef(s.lastSnapshot, snapshot, kind)
	if err := sink.Apply(ctx, ChangeEvent{Kind: kind, Namespace: namespace, Name: name, Change: ChangeTypeUpsert}); err != nil {
		return err
	}
	s.lastHash = hash
	s.lastCategoryHashes = categoryHashMap(categories)
	s.lastSnapshot = snapshot
	return nil
}

type categoryFingerprint struct {
	kind string
	hash uint64
}

func snapshotFingerprint(snapshot Snapshot) (uint64, []categoryFingerprint) {
	cats := []struct {
		kind string
		vals []string
	}{
		{kind: "workload", vals: workloadFingerprintParts(snapshot)},
		{kind: "pod", vals: podFingerprintParts(snapshot)},
		{kind: "service", vals: serviceFingerprintParts(snapshot)},
		{kind: "storage", vals: storageFingerprintParts(snapshot)},
		{kind: "event", vals: eventFingerprintParts(snapshot)},
		{kind: "identity/security", vals: identityFingerprintParts(snapshot)},
	}

	h := fnv.New64a()
	categoryHashes := make([]categoryFingerprint, 0, len(cats))
	for _, cat := range cats {
		sort.Strings(cat.vals)
		joined := strings.Join(cat.vals, "|")
		categoryHashes = append(categoryHashes, categoryFingerprint{kind: cat.kind, hash: hashStrings(cat.kind, joined)})
		_, _ = h.Write([]byte(cat.kind))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(joined))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64(), categoryHashes
}

func hashStrings(parts ...string) uint64 {
	h := fnv.New64a()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

func categoryHashMap(categories []categoryFingerprint) map[string]uint64 {
	out := make(map[string]uint64, len(categories))
	for _, category := range categories {
		out[category.kind] = category.hash
	}
	return out
}

func firstChangedKind(previous map[string]uint64, current []categoryFingerprint) string {
	for _, category := range current {
		if previous[category.kind] != category.hash {
			return category.kind
		}
	}
	return "poll"
}

type refFingerprint struct {
	namespace   string
	name        string
	fingerprint string
}

func changedObjectRef(previous, current Snapshot, kind string) (string, string) {
	switch strings.ToLower(kind) {
	case "workload":
		return firstChangedRef(workloadRefFingerprints(previous), workloadRefFingerprints(current))
	case "pod":
		return firstChangedRef(podRefFingerprints(previous), podRefFingerprints(current))
	case "service":
		return firstChangedRef(serviceRefFingerprints(previous), serviceRefFingerprints(current))
	case "storage":
		return firstChangedRef(storageRefFingerprints(previous), storageRefFingerprints(current))
	case "event":
		return firstChangedRef(eventRefFingerprints(previous), eventRefFingerprints(current))
	case "identity/security":
		return firstChangedRef(identityRefFingerprints(previous), identityRefFingerprints(current))
	default:
		return "", "poll"
	}
}

func firstChangedRef(previous, current []refFingerprint) (string, string) {
	previousByKey := refFingerprintMap(previous)
	currentByKey := refFingerprintMap(current)
	keys := make([]string, 0, len(currentByKey))
	for key := range currentByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		now := currentByKey[key]
		if before, ok := previousByKey[key]; !ok || before.fingerprint != now.fingerprint {
			return now.namespace, now.name
		}
	}

	keys = keys[:0]
	for key := range previousByKey {
		if _, ok := currentByKey[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		before := previousByKey[keys[0]]
		return before.namespace, before.name
	}
	return "", "poll"
}

func refFingerprintMap(in []refFingerprint) map[string]refFingerprint {
	out := make(map[string]refFingerprint, len(in))
	for _, item := range in {
		out[item.namespace+"/"+item.name] = item
	}
	return out
}

func workloadFingerprintParts(snapshot Snapshot) []string {
	refs := workloadRefFingerprints(snapshot)
	out := make([]string, 0, len(refs))
	for _, item := range refs {
		out = append(out, item.fingerprint)
	}
	return out
}

func podFingerprintParts(snapshot Snapshot) []string {
	out := make([]string, 0, len(snapshot.Pods))
	for _, item := range snapshot.Pods {
		out = append(out, podFingerprint(item))
	}
	return out
}

func serviceFingerprintParts(snapshot Snapshot) []string {
	out := make([]string, 0, len(snapshot.Services))
	for _, item := range snapshot.Services {
		out = append(out, serviceFingerprint(item))
	}
	return out
}

func storageFingerprintParts(snapshot Snapshot) []string {
	out := make([]string, 0, len(snapshot.PVCs)+len(snapshot.PVs)+len(snapshot.StorageClasses)+len(snapshot.CSIDrivers))
	for _, item := range snapshot.PVCs {
		out = append(out, pvcFingerprint(item))
	}
	for _, item := range snapshot.PVs {
		out = append(out, pvFingerprint(item))
	}
	for _, item := range snapshot.StorageClasses {
		out = append(out, storageClassFingerprint(item))
	}
	for _, item := range snapshot.CSIDrivers {
		out = append(out, csiDriverFingerprint(item))
	}
	return out
}

func eventFingerprintParts(snapshot Snapshot) []string {
	out := make([]string, 0, len(snapshot.Events))
	for _, item := range snapshot.Events {
		out = append(out, eventFingerprint(item))
	}
	return out
}

func identityFingerprintParts(snapshot Snapshot) []string {
	out := make([]string, 0, len(snapshot.ServiceAccounts)+len(snapshot.RoleBindings)+len(snapshot.ClusterRoleBindings))
	for _, item := range snapshot.ServiceAccounts {
		out = append(out, serviceAccountFingerprint(item))
	}
	for _, item := range snapshot.RoleBindings {
		out = append(out, roleBindingFingerprint(item))
	}
	for _, item := range snapshot.ClusterRoleBindings {
		out = append(out, clusterRoleBindingFingerprint(item))
	}
	return out
}

func workloadRefFingerprints(snapshot Snapshot) []refFingerprint {
	byKey := make(map[string][]string, len(snapshot.Workloads))
	for _, item := range snapshot.Workloads {
		key := item.Metadata.Namespace + "/" + item.Metadata.Name
		byKey[key] = append(byKey[key], workloadFingerprint(item))
	}
	resolver := owner.NewChainResolver("stream", snapshot.Workloads, snapshot.ReplicaSets)
	for _, item := range snapshot.ReplicaSets {
		targets := resolver.ResolveOwnerReferences(item.Metadata.Namespace, item.OwnerReferences)
		if len(targets) == 0 {
			key := item.Metadata.Namespace + "/" + item.Metadata.Name
			byKey[key] = append(byKey[key], replicaSetFingerprint(item))
			continue
		}
		for _, target := range targets {
			key := target.Namespace + "/" + target.Name
			byKey[key] = append(byKey[key], replicaSetFingerprint(item))
		}
	}
	out := make([]refFingerprint, 0, len(byKey))
	for key, fingerprints := range byKey {
		sort.Strings(fingerprints)
		namespace, name, _ := strings.Cut(key, "/")
		out = append(out, refFingerprint{namespace: namespace, name: name, fingerprint: strings.Join(fingerprints, "|")})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].namespace != out[j].namespace {
			return out[i].namespace < out[j].namespace
		}
		return out[i].name < out[j].name
	})
	return out
}

func podRefFingerprints(snapshot Snapshot) []refFingerprint {
	out := make([]refFingerprint, 0, len(snapshot.Pods))
	for _, item := range snapshot.Pods {
		out = append(out, refFingerprint{namespace: item.Metadata.Namespace, name: item.Metadata.Name, fingerprint: podFingerprint(item)})
	}
	return out
}

func serviceRefFingerprints(snapshot Snapshot) []refFingerprint {
	out := make([]refFingerprint, 0, len(snapshot.Services))
	for _, item := range snapshot.Services {
		out = append(out, refFingerprint{namespace: item.Metadata.Namespace, name: item.Metadata.Name, fingerprint: serviceFingerprint(item)})
	}
	return out
}

func storageRefFingerprints(snapshot Snapshot) []refFingerprint {
	out := make([]refFingerprint, 0, len(snapshot.PVCs)+len(snapshot.PVs)+len(snapshot.StorageClasses)+len(snapshot.CSIDrivers))
	for _, item := range snapshot.PVCs {
		out = append(out, refFingerprint{namespace: item.Metadata.Namespace, name: item.Metadata.Name, fingerprint: pvcFingerprint(item)})
	}
	for _, item := range snapshot.PVs {
		out = append(out, refFingerprint{name: item.Metadata.Name, fingerprint: pvFingerprint(item)})
	}
	for _, item := range snapshot.StorageClasses {
		out = append(out, refFingerprint{name: item.Metadata.Name, fingerprint: storageClassFingerprint(item)})
	}
	for _, item := range snapshot.CSIDrivers {
		out = append(out, refFingerprint{name: item.Metadata.Name, fingerprint: csiDriverFingerprint(item)})
	}
	return out
}

func eventRefFingerprints(snapshot Snapshot) []refFingerprint {
	out := make([]refFingerprint, 0, len(snapshot.Events))
	for _, item := range snapshot.Events {
		out = append(out, refFingerprint{namespace: item.Metadata.Namespace, name: item.Metadata.Name, fingerprint: eventFingerprint(item)})
	}
	return out
}

func identityRefFingerprints(snapshot Snapshot) []refFingerprint {
	out := make([]refFingerprint, 0, len(snapshot.ServiceAccounts)+len(snapshot.RoleBindings)+len(snapshot.ClusterRoleBindings))
	for _, item := range snapshot.ServiceAccounts {
		out = append(out, refFingerprint{namespace: item.Metadata.Namespace, name: item.Metadata.Name, fingerprint: serviceAccountFingerprint(item)})
	}
	for _, item := range snapshot.RoleBindings {
		out = append(out, refFingerprint{namespace: item.Metadata.Namespace, name: item.Metadata.Name, fingerprint: roleBindingFingerprint(item)})
	}
	for _, item := range snapshot.ClusterRoleBindings {
		out = append(out, refFingerprint{name: item.Metadata.Name, fingerprint: clusterRoleBindingFingerprint(item)})
	}
	return out
}

func workloadFingerprint(item resources.Workload) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%d/%s/%s/%s",
		item.Metadata.Namespace,
		item.APIVersion,
		item.ControllerKind,
		item.Metadata.Name,
		item.Metadata.UID,
		item.Replicas,
		stringMapFingerprint(item.Metadata.Labels),
		stringMapFingerprint(item.Conditions),
		ownerReferenceFingerprint(item.OwnerReferences),
	)
}

func replicaSetFingerprint(item resources.ReplicaSet) string {
	return fmt.Sprintf("replicaset/%s/%s/%s/%d/%s/%s/%s",
		item.Metadata.Namespace,
		item.Metadata.Name,
		item.Metadata.UID,
		item.Replicas,
		stringMapFingerprint(item.Metadata.Labels),
		stringMapFingerprint(item.Conditions),
		ownerReferenceFingerprint(item.OwnerReferences),
	)
}

func podFingerprint(item resources.Pod) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%s",
		item.Metadata.Namespace,
		item.Metadata.Name,
		item.Metadata.UID,
		item.NodeName,
		item.ServiceAccount,
		item.Phase,
		item.Reason,
		stringMapFingerprint(item.Metadata.Labels),
		ownerReferenceFingerprint(item.OwnerReferences),
		stringSliceFingerprint(item.ContainerImages),
		stringSliceFingerprint(item.ConfigMapRefs)+"/"+stringSliceFingerprint(item.SecretRefs),
		stringSliceFingerprint(item.PVCRefs),
	)
}

func serviceFingerprint(item resources.Service) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		item.Metadata.Namespace,
		item.Metadata.Name,
		item.Metadata.UID,
		stringMapFingerprint(item.Metadata.Labels),
		stringMapFingerprint(item.Selector),
	)
}

func pvcFingerprint(item resources.PVC) string {
	return fmt.Sprintf("pvc/%s/%s/%s/%s/%s/%s",
		item.Metadata.Namespace,
		item.Metadata.Name,
		item.Metadata.UID,
		item.VolumeName,
		item.StorageClassName,
		item.Status,
	)
}

func pvFingerprint(item resources.PV) string {
	return fmt.Sprintf("pv/%s/%s/%s/%s/%s",
		item.Metadata.Name,
		item.Metadata.UID,
		item.StorageClassName,
		item.Status,
		stringMapFingerprint(item.CSI),
	)
}

func storageClassFingerprint(item resources.StorageClass) string {
	return fmt.Sprintf("storageclass/%s/%s/%s/%s/%s",
		item.Metadata.Name,
		item.Metadata.UID,
		item.Provisioner,
		item.ReclaimPolicy,
		item.VolumeBindingMode,
	)
}

func csiDriverFingerprint(item resources.CSIDriver) string {
	return fmt.Sprintf("csidriver/%s/%s",
		item.Metadata.Name,
		item.Metadata.UID,
	)
}

func eventFingerprint(item resources.Event) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s",
		item.Metadata.Namespace,
		item.Metadata.Name,
		item.Metadata.UID,
		item.InvolvedKind,
		item.InvolvedName,
		item.Reason,
		item.Message,
	)
}

func serviceAccountFingerprint(item resources.ServiceAccount) string {
	return fmt.Sprintf("sa/%s/%s/%s", item.Metadata.Namespace, item.Metadata.Name, item.Metadata.UID)
}

func roleBindingFingerprint(item resources.RoleBinding) string {
	return fmt.Sprintf("rb/%s/%s/%s/%s/%s",
		item.Metadata.Namespace,
		item.Metadata.Name,
		item.Metadata.UID,
		item.RoleRef,
		subjectFingerprint(item.SubjectKinds, item.SubjectNames, item.SubjectNamespaces),
	)
}

func clusterRoleBindingFingerprint(item resources.ClusterRoleBinding) string {
	return fmt.Sprintf("crb/%s/%s/%s/%s",
		item.Metadata.Name,
		item.Metadata.UID,
		item.RoleRef,
		subjectFingerprint(item.SubjectKinds, item.SubjectNames, item.SubjectNamespaces),
	)
}

func stringMapFingerprint(in map[string]string) string {
	if len(in) == 0 {
		return ""
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+in[key])
	}
	return strings.Join(parts, ",")
}

func stringSliceFingerprint(in []string) string {
	if len(in) == 0 {
		return ""
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return strings.Join(out, ",")
}

func subjectFingerprint(kinds, names, namespaces []string) string {
	maxLen := len(kinds)
	if len(names) > maxLen {
		maxLen = len(names)
	}
	if len(namespaces) > maxLen {
		maxLen = len(namespaces)
	}
	if maxLen == 0 {
		return ""
	}
	parts := make([]string, 0, maxLen)
	for index := 0; index < maxLen; index++ {
		parts = append(parts, fmt.Sprintf("%s/%s/%s", sliceValue(kinds, index), sliceValue(names, index), sliceValue(namespaces, index)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func sliceValue(in []string, index int) string {
	if index < 0 || index >= len(in) {
		return ""
	}
	return in[index]
}

func ownerReferenceFingerprint(in []resources.OwnerReference) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for _, ref := range in {
		parts = append(parts, fmt.Sprintf("%s/%s/%s/%s/%t", ref.APIVersion, ref.Kind, ref.Name, ref.UID, ref.Controller))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
