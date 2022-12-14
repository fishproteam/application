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

package v1beta1

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var applicationlog = logf.Log.WithName("application-resource")

func (r *Application) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-applications-app-io-v1beta1-application,mutating=true,failurePolicy=fail,sideEffects=None,groups=applications.app.io,resources=applications,verbs=create;update,versions=v1beta1,name=mapplication.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Application{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Application) Default() {
	applicationlog.Info("default", "name", r.Name)

	// set default revision history
	defaultHistoryLimit := int32(10)
	if r.Spec.RevisionHistoryLimit == nil {
		r.Spec.RevisionHistoryLimit = &defaultHistoryLimit
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-applications-app-io-v1beta1-application,mutating=false,failurePolicy=fail,sideEffects=None,groups=applications.app.io,resources=applications,verbs=create;update,versions=v1beta1,name=vapplication.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Application{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Application) ValidateCreate() error {
	applicationlog.Info("validate create", "name", r.Name)

	return r.validateApplication()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Application) ValidateUpdate(old runtime.Object) error {
	applicationlog.Info("validate update", "name", r.Name)

	return r.validateApplication()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Application) ValidateDelete() error {
	applicationlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

func (r *Application) validateApplication() error {
	var errs field.ErrorList

	if *r.Spec.RevisionHistoryLimit > 50 {
		applicationlog.Info("c. Invalid revisionHistoryLimit")

		err := field.Invalid(field.NewPath("spec").Child("revisionHistoryLimit"),
			*r.Spec.RevisionHistoryLimit,
			"d. must be less than 100")

		errs = append(errs, err)
	}

	if len(r.Spec.Resources) > 50 {
		applicationlog.Info("c. Invalid resources")

		err := field.Invalid(field.NewPath("spec").Child("resources"),
			fmt.Sprintf("the number of resources is %d", len(r.Spec.Resources)),
			"d. the number of resources must be less than 50")

		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return apierrors.NewInvalid(
			GroupVersion.WithKind("Application").GroupKind(),
			r.Name,
			errs)
	}
	return nil
}
