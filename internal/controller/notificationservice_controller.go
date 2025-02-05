/*
Copyright 2024.

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
	"fmt"

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NotificationServiceReconciler reconciles a NotificationService object
type NotificationServiceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=konflux-ci.com,resources=notificationservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=konflux-ci.com,resources=notificationservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=konflux-ci.com,resources=notificationservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile will monitor the pipelinerun, extract its result and send it as a webhook
// When a pipelinerun is created, it will add a finalizer to it so we will be able to extract the results
// After a pipelinerun ends successfully, the results will be extracted from it and will be sent as a webhook,
// An annotation will be added to mark this pipelinerun as handled and the finalizer will be rmoved
// to allow the deletion of this pipelinerun
func (r *NotificationServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := r.Log.WithValues("pipelinerun", req.NamespacedName)
	pipelineRun := &tektonv1.PipelineRun{}

	err := r.Get(ctx, req.NamespacedName, pipelineRun)
	if err != nil {
		logger.Error(err, "Failed to get pipelineRun for", "req", req.NamespacedName)
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if IsAnnotationExistInPipelineRun(pipelineRun, NotificationPipelineRunAnnotation, NotificationPipelineRunAnnotationValue) &&
		!IsFinalizerExistInPipelineRun(pipelineRun, NotificationPipelineRunFinalizer) {
		logger.Info("No need to reconcile pipelinerun %s", pipelineRun.Name)
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling PipelineRun", "Name", pipelineRun.Name)
	if !IsFinalizerExistInPipelineRun(pipelineRun, NotificationPipelineRunFinalizer) &&
		!IsAnnotationExistInPipelineRun(pipelineRun, NotificationPipelineRunAnnotation, NotificationPipelineRunAnnotationValue) {
		err = AddFinalizerToPipelineRun(ctx, pipelineRun, r, NotificationPipelineRunFinalizer)
		if err != nil {
			logger.Error(err, "Failed to add finalizer to pipelinerun ", pipelineRun.Name)
		}
	}

	if IsPipelineRunEndedSuccessfully(pipelineRun) &&
		!IsAnnotationExistInPipelineRun(pipelineRun, NotificationPipelineRunAnnotation, NotificationPipelineRunAnnotationValue) {
		results, err := GetResultsFromPipelineRun(pipelineRun)
		if err != nil {
			logger.Error(err, "Failed to get results for pipelineRun ", pipelineRun.Name)
		} else {
			fmt.Printf("Results for pipelinerun %s are: %s\n", pipelineRun.Name, results)
			err = AddAnnotationToPipelineRun(ctx, pipelineRun, r, NotificationPipelineRunAnnotation, NotificationPipelineRunAnnotationValue)
			if err != nil {
				logger.Error(err, "Failed to add annotation")
			}
		}
	}

	if IsPipelineRunEndedSuccessfully(pipelineRun) &&
		IsAnnotationExistInPipelineRun(pipelineRun, NotificationPipelineRunAnnotation, NotificationPipelineRunAnnotationValue) {
		err = RemoveFinalizerFromPipelineRun(ctx, pipelineRun, r, NotificationPipelineRunFinalizer)
		if err != nil {
			logger.Error(err, "Failed to remove finalizer to pipelinerun ", pipelineRun.Name)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NotificationServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1.PipelineRun{}).
		Complete(r)
}
