/*
Copyright 2025.

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

package controller

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	loadtestv1 "github.com/seredavin/loadtest-operator/api/v1"
)

// LoadManagerReconciler reconciles a LoadManager object
type LoadManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=loadtest.gitopscd.ru,resources=loadmanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=loadtest.gitopscd.ru,resources=loadmanagers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=loadtest.gitopscd.ru,resources=loadmanagers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LoadManager object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *LoadManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var loadManager loadtestv1.LoadManager
	if err := r.Get(ctx, req.NamespacedName, &loadManager); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling LoadManager",
		"Name", loadManager.Name,
		"ChildCount", loadManager.Spec.ChildCount,
		"StatusUpdateTimeout", loadManager.Spec.StatusUpdateTimeout,
		"PayloadSize", loadManager.Spec.PayloadSize,
		"UpdateFrequency", loadManager.Spec.UpdateFrequency,
		"Mode", loadManager.Spec.Mode,
		"MaxConcurrent", loadManager.Spec.MaxConcurrent,
	)

	// 1. List all LoadWorkers owned by this LoadManager
	var workerList loadtestv1.LoadWorkerList
	if err := r.List(ctx, &workerList, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "unable to list LoadWorkers")
		return ctrl.Result{}, err
	}

	// Filter only those with ownerReference to this LoadManager
	var ownedWorkers []loadtestv1.LoadWorker
	for _, w := range workerList.Items {
		for _, owner := range w.OwnerReferences {
			if owner.Kind == "LoadManager" && owner.UID == loadManager.UID {
				ownedWorkers = append(ownedWorkers, w)
				break
			}
		}
	}

	desiredCount := loadManager.Spec.ChildCount
	currentCount := len(ownedWorkers)

	// 2. Create missing LoadWorkers
	for i := currentCount; i < desiredCount; i++ {
		worker := &loadtestv1.LoadWorker{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: loadManager.Name + "-worker-",
				Namespace:    loadManager.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(&loadManager, loadtestv1.GroupVersion.WithKind("LoadManager")),
				},
			},
			Spec: loadtestv1.LoadWorkerSpec{
				ChildCount:          loadManager.Spec.ChildCount,
				StatusUpdateTimeout: loadManager.Spec.StatusUpdateTimeout,
				PayloadSize:         loadManager.Spec.PayloadSize,
				UpdateFrequency:     loadManager.Spec.UpdateFrequency,
				Mode:                loadManager.Spec.Mode,
				MaxConcurrent:       loadManager.Spec.MaxConcurrent,
			},
		}
		if err := r.Create(ctx, worker); err != nil {
			log.Error(err, "failed to create LoadWorker")
			return ctrl.Result{}, err
		}
		log.Info("Created LoadWorker", "Name", worker.Name)
	}

	// 3. Delete excess LoadWorkers
	if currentCount > desiredCount {
		for i := desiredCount; i < currentCount; i++ {
			w := ownedWorkers[i]
			if err := r.Delete(ctx, &w); err != nil {
				log.Error(err, "failed to delete LoadWorker", "Name", w.Name)
				return ctrl.Result{}, err
			}
			log.Info("Deleted LoadWorker", "Name", w.Name)
		}
	}

	// 4. Update existing LoadWorkers if spec differs
	for i := 0; i < min(currentCount, desiredCount); i++ {
		w := ownedWorkers[i]
		updated := false

		if w.Spec.ChildCount != loadManager.Spec.ChildCount {
			w.Spec.ChildCount = loadManager.Spec.ChildCount
			updated = true
		}
		if w.Spec.StatusUpdateTimeout != loadManager.Spec.StatusUpdateTimeout {
			w.Spec.StatusUpdateTimeout = loadManager.Spec.StatusUpdateTimeout
			updated = true
		}
		if w.Spec.PayloadSize != loadManager.Spec.PayloadSize {
			w.Spec.PayloadSize = loadManager.Spec.PayloadSize
			updated = true
		}
		if w.Spec.UpdateFrequency != loadManager.Spec.UpdateFrequency {
			w.Spec.UpdateFrequency = loadManager.Spec.UpdateFrequency
			updated = true
		}
		if w.Spec.Mode != loadManager.Spec.Mode {
			w.Spec.Mode = loadManager.Spec.Mode
			updated = true
		}
		if w.Spec.MaxConcurrent != loadManager.Spec.MaxConcurrent {
			w.Spec.MaxConcurrent = loadManager.Spec.MaxConcurrent
			updated = true
		}

		if updated {
			if err := r.Update(ctx, &w); err != nil {
				log.Error(err, "failed to update LoadWorker", "Name", w.Name)
				return ctrl.Result{}, err
			}
			log.Info("Updated LoadWorker", "Name", w.Name)
		}
	}

	return ctrl.Result{}, nil
}

// min helper
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&loadtestv1.LoadManager{}).
		Named("loadmanager").
		Complete(r)
}
