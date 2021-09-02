/*
Copyright 2021 Stefan Prodan
Copyright 2021 The Flux authors

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

package resmgr

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Diff performs a server-side apply dry-un and returns the fields that changed.
// If the diff contains Kubernetes Secrets, the data values are masked.
func (kc *ResourceManager) Diff(ctx context.Context, object *unstructured.Unstructured) (*ChangeSetEntry, error) {
	existingObject := object.DeepCopy()
	_ = kc.kubeClient.Get(ctx, client.ObjectKeyFromObject(object), existingObject)

	dryRunObject := object.DeepCopy()
	if err := kc.dryRunApply(ctx, dryRunObject); err != nil {
		return nil, kc.validationError(dryRunObject, err)
	}

	if dryRunObject.GetResourceVersion() == "" {
		return kc.changeSetEntry(dryRunObject, CreatedAction), nil
	}

	if kc.hasDrifted(existingObject, dryRunObject) {
		cse := kc.changeSetEntry(object, ConfiguredAction)

		unstructured.RemoveNestedField(dryRunObject.Object, "metadata", "managedFields")
		unstructured.RemoveNestedField(existingObject.Object, "metadata", "managedFields")

		if dryRunObject.GetKind() == "Secret" {
			d, err := kc.fmt.MaskSecret(dryRunObject, "******")
			if err != nil {
				return nil, fmt.Errorf("masking secret data failed, error: %w", err)
			}
			dryRunObject = d
			ex, err := kc.fmt.MaskSecret(existingObject, "*****")
			if err != nil {
				return nil, fmt.Errorf("masking secret data failed, error: %w", err)
			}
			existingObject = ex
		}

		d, _ := yaml.Marshal(dryRunObject)
		e, _ := yaml.Marshal(existingObject)
		cse.Diff = cmp.Diff(string(e), string(d))

		return cse, nil
	}

	return kc.changeSetEntry(dryRunObject, UnchangedAction), nil
}

// hasDrifted detects changes to metadata labels, metadata annotations, spec and webhooks.
func (kc *ResourceManager) hasDrifted(existingObject, dryRunObject *unstructured.Unstructured) bool {
	if dryRunObject.GetResourceVersion() == "" {
		return true
	}

	if !apiequality.Semantic.DeepDerivative(dryRunObject.GetLabels(), existingObject.GetLabels()) {
		return true

	}

	if !apiequality.Semantic.DeepDerivative(dryRunObject.GetAnnotations(), existingObject.GetAnnotations()) {
		return true
	}

	if _, ok := existingObject.Object["spec"]; ok {
		if !apiequality.Semantic.DeepDerivative(dryRunObject.Object["spec"], existingObject.Object["spec"]) {
			return true
		}
	} else if _, ok := existingObject.Object["webhooks"]; ok {
		if !apiequality.Semantic.DeepDerivative(dryRunObject.Object["webhooks"], existingObject.Object["webhooks"]) {
			return true
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(dryRunObject.Object, existingObject.Object) {
			return true
		}
	}

	return false
}

// validationError formats the given error and hides sensitive data
// if the error was caused by an invalid Kubernetes secrets.
func (kc *ResourceManager) validationError(object *unstructured.Unstructured, err error) error {
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%s namespace not specified, error: %w", kc.fmt.Unstructured(object), err)
	}

	if object.GetKind() == "Secret" {
		return fmt.Errorf("%s is invalid, error: data values must be of type string", kc.fmt.Unstructured(object))
	}

	return fmt.Errorf("%s is invalid, error: %w", kc.fmt.Unstructured(object), err)

}
