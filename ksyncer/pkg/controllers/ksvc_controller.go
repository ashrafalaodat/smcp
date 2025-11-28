package controllers

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/ashrafalaodat/smcp/ksyncer/pkg/annotations"
	"github.com/ashrafalaodat/smcp/ksyncer/pkg/hash"
	"github.com/ashrafalaodat/smcp/ksyncer/pkg/registry"
)

// KServiceReconciler keeps Knative services in sync with the registry.
type KServiceReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Registry     *registry.Client
	RequeueDelay time.Duration
}

const (
	finalizerName       = "ksyncer/finalizer"
	eventReasonSyncFail = "SyncFailed"
	eventReasonSynced   = "Synced"
	eventReasonMissing  = "MissingDescription"
)

// Reconcile implements the reconciliation loop.
func (r *KServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	svc := &unstructured.Unstructured{}
	svc.SetGroupVersionKind(servingGVK)

	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !svc.GetDeletionTimestamp().IsZero() {
		if containsString(svc.GetFinalizers(), finalizerName) {
			if err := r.handleDelete(ctx, svc); err != nil {
				r.Recorder.Event(svc, corev1.EventTypeWarning, eventReasonSyncFail, err.Error())
				return ctrl.Result{}, err
			}
			if err := r.patchFinalizers(ctx, svc, removeString); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer present.
	if !containsString(svc.GetFinalizers(), finalizerName) {
		if err := r.patchFinalizers(ctx, svc, addString); err != nil {
			return ctrl.Result{}, err
		}
	}

    ann := mergeAnnotations(svc)
	desc := ann[annotations.KeyDescription]
	if desc == "" {
		r.Recorder.Event(svc, corev1.EventTypeWarning, eventReasonMissing, "ksyncer/description annotation required")
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, nil
	}

	visibility := ann[annotations.KeyVisibility]
	if visibility == "" {
		visibility = "private"
	}

	payload := registry.ToolPayload{
		Owner:       svc.GetNamespace(),
		Name:        svc.GetName(),
		Visibility:  visibility,
		Description: desc,
	}

	lastHash, _, _ := unstructured.NestedString(svc.Object, "status", "ksyncerLastHash")
	currentHash := hash.Map(map[string]string{
		"description": payload.Description,
		"visibility":  payload.Visibility,
	})

	if lastHash == currentHash {
		return ctrl.Result{}, nil
	}

	if err := r.Registry.Upsert(ctx, payload); err != nil {
		r.Recorder.Event(svc, corev1.EventTypeWarning, eventReasonSyncFail, err.Error())
		return ctrl.Result{}, err
	}

	// Persist last hash in status.
	if err := r.patchStatusHash(ctx, svc, currentHash); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(svc, corev1.EventTypeNormal, eventReasonSynced, "synced to registry")
	logger.Info("synced service", "namespace", svc.GetNamespace(), "name", svc.GetName())

	return ctrl.Result{}, nil
}

func (r *KServiceReconciler) handleDelete(ctx context.Context, svc *unstructured.Unstructured) error {
	if err := r.Registry.Delete(ctx, svc.GetNamespace(), svc.GetName()); err != nil {
		return err
	}
	r.Recorder.Event(svc, corev1.EventTypeNormal, eventReasonSynced, "deleted from registry")
	return nil
}

func (r *KServiceReconciler) patchFinalizers(ctx context.Context, svc *unstructured.Unstructured, mutate func([]string, string) []string) error {
	patched := svc.DeepCopy()
	patched.SetFinalizers(mutate(patched.GetFinalizers(), finalizerName))
	return r.Patch(ctx, patched, client.MergeFrom(svc))
}

func (r *KServiceReconciler) patchAnnotation(ctx context.Context, svc *unstructured.Unstructured, key, value string) error {
	patched := svc.DeepCopy()
	ann := patched.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[key] = value
	patched.SetAnnotations(ann)
	return r.Patch(ctx, patched, client.MergeFrom(svc))
}

func (r *KServiceReconciler) patchStatusHash(ctx context.Context, svc *unstructured.Unstructured, hash string) error {
	patched := svc.DeepCopy()
	if err := unstructured.SetNestedField(patched.Object, hash, "status", "ksyncerLastHash"); err != nil {
		return err
	}
	return r.Status().Patch(ctx, patched, client.MergeFrom(svc))
}

// mergeAnnotations combines top-level and spec.template.metadata.annotations.
func mergeAnnotations(svc *unstructured.Unstructured) map[string]string {
    out := map[string]string{}

    if top := svc.GetAnnotations(); top != nil {
        for k, v := range top {
            out[k] = v
        }
    }

    if tmpl, found, _ := unstructured.NestedStringMap(svc.Object, "spec", "template", "metadata", "annotations"); found {
        for k, v := range tmpl {
            out[k] = v
        }
    }
    return out
}

// SetupWithManager wires the controller.
func (r *KServiceReconciler) SetupWithManager(mgr ctrl.Manager, concurrency int) error {
	if concurrency <= 0 {
		concurrency = 1
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return true },
		DeleteFunc: func(e event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true
			}
			return annotationsChanged(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
		},
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(servingGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(obj).
		WithEventFilter(pred).
		WithOptions(controller.Options{MaxConcurrentReconciles: concurrency}).
		Complete(r)
}

func annotationsChanged(old, new map[string]string) bool {
	for _, key := range annotations.RelevantKeys {
		if old[key] != new[key] {
			return true
		}
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func addString(list []string, s string) []string {
	if containsString(list, s) {
		return list
	}
	return append(list, s)
}

func removeString(list []string, s string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item != s {
			out = append(out, item)
		}
	}
	return out
}

// Ensure KServiceReconciler implements reconcile.Reconciler.
var _ interface {
	Reconcile(context.Context, ctrl.Request) (ctrl.Result, error)
} = &KServiceReconciler{}

var (
	servingGVK = schema.GroupVersionKind{Group: "serving.knative.dev", Version: "v1", Kind: "Service"}
)
