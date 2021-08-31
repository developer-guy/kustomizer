/*
Copyright 2021 Stefan Prodan

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

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/stefanprodan/kustomizer/pkg/resmgr"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply Kubernetes manifests and Kustomize overlays using server-side apply.",
	RunE:  runApplyCmd,
}

type applyFlags struct {
	filename           []string
	kustomize          string
	inventoryName      string
	inventoryNamespace string
	wait               bool
	force              bool
	prune              bool
}

var applyArgs applyFlags

func init() {
	applyCmd.Flags().StringSliceVarP(&applyArgs.filename, "filename", "f", nil, "path to Kubernetes manifest(s)")
	applyCmd.Flags().StringVarP(&applyArgs.kustomize, "kustomize", "k", "", "process a kustomization directory (can't be used together with -f)")
	applyCmd.Flags().BoolVar(&applyArgs.wait, "wait", false, "wait for the applied Kubernetes objects to become ready")
	applyCmd.Flags().BoolVar(&applyArgs.force, "force", false, "recreate objects that contain immutable fields changes")
	applyCmd.Flags().BoolVar(&applyArgs.prune, "prune", false, "delete stale objects")
	applyCmd.Flags().StringVarP(&applyArgs.inventoryName, "inventory-name", "i", "", "inventory configmap name")
	applyCmd.Flags().StringVar(&applyArgs.inventoryNamespace, "inventory-namespace", "default", "inventory configmap namespace")

	rootCmd.AddCommand(applyCmd)
}

func runApplyCmd(cmd *cobra.Command, args []string) error {
	if applyArgs.kustomize == "" && len(applyArgs.filename) == 0 {
		return fmt.Errorf("-f or -k is required")
	}
	if applyArgs.inventoryName == "" {
		return fmt.Errorf("--inventory-name is required")
	}
	if applyArgs.inventoryNamespace == "" {
		return fmt.Errorf("--inventory-namespace is required")
	}

	objects, err := buildManifests(applyArgs.kustomize, applyArgs.filename)
	if err != nil {
		return err
	}

	newInventory, err := inventoryMgr.Record(objects)
	if err != nil {
		return fmt.Errorf("creating inventory failed, error: %w", err)
	}

	resMgr, err := resmgr.NewResourceManager(rootArgs.kubeconfig, rootArgs.kubecontext, PROJECT)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	for _, object := range objects {
		change, err := resMgr.Apply(ctx, object, applyArgs.force)
		if err != nil {
			return err
		}
		logger.Println(change.String())
	}

	staleObjects, err := inventoryMgr.GetStaleObjects(ctx, resMgr.KubeClient(), newInventory, applyArgs.inventoryName, applyArgs.inventoryNamespace)
	if err != nil {
		return fmt.Errorf("inventory query failed, error: %w", err)
	}

	err = inventoryMgr.Store(ctx, resMgr.KubeClient(), newInventory, applyArgs.inventoryName, applyArgs.inventoryNamespace)
	if err != nil {
		return fmt.Errorf("inventory apply failed, error: %w", err)
	}

	if applyArgs.prune && len(staleObjects) > 0 {
		changeSet, err := resMgr.DeleteAll(ctx, staleObjects)
		if err != nil {
			return fmt.Errorf("prune failed, error: %w", err)
		}
		for _, change := range changeSet.Entries {
			logger.Println(change.String())
		}
	}

	if applyArgs.wait {
		logger.Println("waiting for resources to become ready...")

		err = resMgr.Wait(objects, 2*time.Second, rootArgs.timeout)
		if err != nil {
			return err
		}

		if applyArgs.prune && len(staleObjects) > 0 {
			err = resMgr.WaitForTermination(staleObjects, 2*time.Second, rootArgs.timeout)
			if err != nil {
				return fmt.Errorf("wating for termination failed, error: %w", err)
			}
		}

		logger.Println("all resources are ready")
	}

	return nil
}
