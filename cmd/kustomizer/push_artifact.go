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
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"

	"github.com/stefanprodan/kustomizer/pkg/registry"
)

var pushArtifactCmd = &cobra.Command{
	Use:   "artifact",
	Short: "Push uploads Kubernetes manifests to a container registry.",
	Long: `The push command scans the given path for Kubernetes manifests or Kustomize overlays,
builds the manifests into a multi-doc YAML, packages the YAML file into an OCI artifact and
pushes the image to the container registry.
The push command uses the credentials from '~/.docker/config.json'.`,
	Example: `  kustomizer push artifact <oci url> -k <overlay path> [-f <dir path>|<file path>]

  # Build Kubernetes plain manifests and push the resulting multi-doc YAML to Docker Hub
  kustomizer push artifact oci://docker.io/user/repo:v1.0.0 -f ./deploy/manifests

  # Build a Kustomize overlay and push the resulting multi-doc YAML to GitHub Container Registry
  kustomizer push artifact oci://ghcr.io/user/repo:v1.0.0 -k ./deploy/production 

  # Push to a local registry
  kustomizer push artifact oci://localhost:5000/repo:latest -f ./deploy/manifests 
`,
	RunE: runPushArtifactCmd,
}

type pushArtifactFlags struct {
	filename  []string
	kustomize string
	patch     []string
}

var pushArtifactArgs pushArtifactFlags

func init() {
	pushArtifactCmd.Flags().StringSliceVarP(&pushArtifactArgs.filename, "filename", "f", nil,
		"Path to Kubernetes manifest(s). If a directory is specified, then all manifests in the directory tree will be processed recursively.")
	pushArtifactCmd.Flags().StringVarP(&pushArtifactArgs.kustomize, "kustomize", "k", "",
		"Path to a directory that contains a kustomization.yaml.")
	pushArtifactCmd.Flags().StringSliceVarP(&pushArtifactArgs.patch, "patch", "p", nil,
		"Path to a kustomization file that contains a list of patches.")

	pushCmd.AddCommand(pushArtifactCmd)
}

func runPushArtifactCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify an artifact name e.g. 'oci://docker.io/user/repo:tag'")
	}

	if pushArtifactArgs.kustomize == "" && len(pushArtifactArgs.filename) == 0 {
		return fmt.Errorf("-f or -k is required")
	}

	url, err := registry.ParseURL(args[0])
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	logger.Println("building manifests...")
	objects, _, err := buildManifests(ctx, pushArtifactArgs.kustomize, pushArtifactArgs.filename, nil, pushArtifactArgs.patch)
	if err != nil {
		return err
	}

	sort.Sort(ssa.SortableUnstructureds(objects))

	for _, object := range objects {
		rootCmd.Println(ssa.FmtUnstructured(object))
	}

	yml, err := ssa.ObjectsToYAML(objects)
	if err != nil {
		return err
	}

	logger.Println("pushing image", url)
	digest, err := registry.Push(ctx, url, yml, &registry.Metadata{
		Version:  VERSION,
		Checksum: fmt.Sprintf("%x", sha256.Sum256([]byte(yml))),
		Created:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("pushing image failed: %w", err)
	}

	logger.Println("published digest", digest)

	return nil
}
