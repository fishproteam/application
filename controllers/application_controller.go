/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1beta1 "github.com/application-io/application/api/v1beta1"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=applications.app.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=applications.app.io,resources=applications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=applications.app.io,resources=applications/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("start reconcile application")

	var app appv1beta1.Application
	err := r.Get(ctx, req.NamespacedName, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if app.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	_, olds, err := r.constructHistory(ctx, &app)

	resources, errs := r.ensureResources(ctx, &app)
	newApplicationStatus := r.getNewApplicationStatus(ctx, &app, resources, &errs)

	newApplicationStatus.ObservedGeneration = app.Generation
	if !equality.Semantic.DeepEqual(newApplicationStatus, &app.Status) {
		err = r.updateApplicationStatus(ctx, req.NamespacedName, newApplicationStatus)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.cleanupHistory(ctx, &app, olds)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to clean up revisions of Application: %v", err)
	}

	return ctrl.Result{}, nil
}

// getNewApplicationStatus get the application status
func (r *ApplicationReconciler) getNewApplicationStatus(ctx context.Context, app *appv1beta1.Application, resources []*unstructured.Unstructured, errList *[]error) *appv1beta1.ApplicationStatus {
	objectStatuses := r.resourceStatuses(ctx, resources, errList)
	errs := utilerrors.NewAggregate(*errList)

	aggReady, countReady := aggregateReady(objectStatuses)

	newApplicationStatus := app.Status.DeepCopy()
	newApplicationStatus.ResourceStatuses = objectStatuses
	newApplicationStatus.ComponentsReady = fmt.Sprintf("%d/%d", countReady, len(objectStatuses))
	if errs != nil {
		setReadyUnknownCondition(newApplicationStatus, "ComponentsReadyUnknown", "failed to aggregate all components' statuses, check the Error condition for details")
	} else if aggReady {
		setReadyCondition(newApplicationStatus, "ComponentsReady", "all components ready")
	} else {
		setNotReadyCondition(newApplicationStatus, "ComponentsNotReady", fmt.Sprintf("%d components not ready", len(objectStatuses)-countReady))
	}

	if errs != nil {
		setErrorCondition(newApplicationStatus, "ErrorSeen", errs.Error())
	} else {
		clearErrorCondition(newApplicationStatus)
	}

	return newApplicationStatus
}

// updateApplicationStatus update application status
func (r *ApplicationReconciler) updateApplicationStatus(ctx context.Context, nn types.NamespacedName, status *appv1beta1.ApplicationStatus) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		original := &appv1beta1.Application{}
		if err := r.Get(ctx, nn, original); err != nil {
			return err
		}
		original.Status = *status
		if err := r.Client.Status().Update(ctx, original); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update status of Application %s/%s: %v", nn.Namespace, nn.Name, err)
	}
	return nil
}

// ensureResources reconcile the application's resources
func (r *ApplicationReconciler) ensureResources(ctx context.Context, app *appv1beta1.Application) ([]*unstructured.Unstructured, []error) {

	logger := log.FromContext(ctx)
	logger.Info("start ensure resources")

	var (
		errs      []error
		resources []*unstructured.Unstructured
	)
	for _, template := range app.Spec.Resources {
		resource := &unstructured.Unstructured{}
		if err := resource.UnmarshalJSON(template.Raw); err != nil {
			errs = append(errs, err)
			continue
		}
		resource.SetNamespace(app.Namespace)
		controllerutil.SetControllerReference(app, resource, r.Scheme)
		resources = append(resources, resource)
	}

	for _, resource := range resources {
		logger.Info("ensoure resource", "resource", fmt.Sprintf("%s/%s", resource.GetKind(), resource.GetName()))
		applied := resource.DeepCopy()
		if err := r.Get(ctx, types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()}, applied); err != nil {
			if apierrors.IsNotFound(err) {
				if err := r.Create(ctx, resource); err != nil {
					logger.Error(err, "failed to create resource")
					errs = append(errs, err)
					continue
				}
			}
			errs = append(errs, err)
		} else {
			// if the object not controlled by application and had owners, conflict
			if !metav1.IsControlledBy(applied, app) && len(applied.GetOwnerReferences()) > 0 {
				errs = append(errs, fmt.Errorf("the resource %s/%s is controlled by other resource", applied.GetKind(), applied.GetName()))
				continue
			}
			resource.SetResourceVersion(applied.GetResourceVersion())
			if err := r.Patch(ctx, resource, client.Merge, &client.PatchOptions{}); err != nil {
				logger.Error(err, "failed to patch resource")
				errs = append(errs, err)
			}
		}
	}
	return resources, errs
}

// resourceStatuses get the application resources status
func (r *ApplicationReconciler) resourceStatuses(ctx context.Context, resources []*unstructured.Unstructured, errs *[]error) []appv1beta1.ResourceStatus {
	logger := log.FromContext(ctx)

	var resourceStatuses []appv1beta1.ResourceStatus
	for _, resource := range resources {
		resourceReference := appv1beta1.ResourceReference{
			APIVersion:      resource.GetAPIVersion(),
			Kind:            resource.GetKind(),
			Namespace:       resource.GetNamespace(),
			Name:            resource.GetName(),
			ResourceVersion: resource.GetResourceVersion(),
		}
		s, err := status(resource)
		if err != nil {
			logger.Error(err, "unable to compute status for resource", "gvk", resource.GroupVersionKind().String(),
				"namespace", resource.GetNamespace(), "name", resource.GetName())
			*errs = append(*errs, err)
		}
		resourceStatuses = append(resourceStatuses, appv1beta1.ResourceStatus{
			Resource:       resourceReference,
			ComputedStatus: s,
		})
	}
	return resourceStatuses
}

func aggregateReady(objectStatuses []appv1beta1.ResourceStatus) (bool, int) {
	countReady := 0
	for _, os := range objectStatuses {
		if os.ComputedStatus == StatusReady {
			countReady++
		}
	}
	if countReady == len(objectStatuses) {
		return true, countReady
	}
	return false, countReady
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1beta1.Application{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
