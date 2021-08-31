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

// Package inventory contains utilities for keeping a record of Kubernetes objects applied on a cluster.
//
// The InventoryManager performs the following actions:
// - decodes raw manifests (YAML & JSON) into Kubernetes objects
// - records the objects metadata and stores the inventory in a Kubernetes ConfigMap
// - determines which objects are subject to garbage collection
package inventory
