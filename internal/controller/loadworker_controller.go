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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	loadtestv1 "github.com/seredavin/loadtest-operator/api/v1"
)

// LoadWorkerReconciler reconciles a LoadWorker object
type LoadWorkerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=loadtest.gitopscd.ru,resources=loadworkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=loadtest.gitopscd.ru,resources=loadworkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=loadtest.gitopscd.ru,resources=loadworkers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LoadWorker object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *LoadWorkerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var worker loadtestv1.LoadWorker
	if err := r.Get(ctx, req.NamespacedName, &worker); err != nil {
		log.Error(err, "unable to fetch LoadWorker")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	spec := worker.Spec
	if spec.Mode != "auto" {
		return ctrl.Result{RequeueAfter: 0}, nil
	}

	// Генерируем payload нужного размера
	payload := make([]byte, spec.PayloadSize)
	for i := range payload {
		payload[i] = byte('A' + i%26)
	}

	// Параллельное обновление статуса
	tasks := spec.MaxConcurrent
	if tasks < 1 {
		tasks = 1
	}
	updateFreq := spec.UpdateFrequency
	if updateFreq < 1 {
		updateFreq = 1000
	}

	// Пример: обновим статус ChildCount раз с заданной частотой
	for i := 0; i < spec.ChildCount; i++ {
		go func(idx int) {
			// Можно добавить поля в статус, например, PayloadHash или Progress
			// status := worker.Status
			// status.Progress = float32(idx+1) / float32(spec.ChildCount)
			// status.PayloadHash = fmt.Sprintf("%x", sha256.Sum256(payload))
			err := r.Status().Update(ctx, &worker)
			if err != nil {
				log.Error(err, "failed to update status")
			}
		}(i)
		time.Sleep(time.Duration(updateFreq) * time.Millisecond)
	}

	return ctrl.Result{RequeueAfter: time.Duration(spec.StatusUpdateTimeout) * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadWorkerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&loadtestv1.LoadWorker{}).
		Named("loadworker").
		Complete(r)
}
