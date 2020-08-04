/*
 * This file is part of MinIO Operator
 * Copyright (C) 2020, MinIO, Inc.
 *
 * This code is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, version 3,
 * as published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License, version 3,
 * along with this program.  If not, see <http://www.gnu.org/licenses/>
 *
 */

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/minio/kubectl-minio/cmd/helpers"
	"github.com/minio/kubectl-minio/cmd/resources"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"
	operatorv1 "github.com/minio/operator/pkg/client/clientset/versioned"
	"github.com/spf13/cobra"
)

const (
	addDesc = `
'add' command adds zones to a MinIO tenant`
	addVolumeExample = `  kubectl minio tenant volume add --name tenant1 --servers 4 --volumes 32 --capacity 32Ti --namespace tenant1-ns`
)

type volumeAddCmd struct {
	out        io.Writer
	errOut     io.Writer
	output     bool
	tenantOpts resources.TenantOptions
}

func newVolumeAddCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	v := &volumeAddCmd{out: out, errOut: errOut}

	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add volumes to existing tenant",
		Long:    addDesc,
		Example: addVolumeExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := v.validate(); err != nil {
				return err
			}
			return v.run()
		},
	}

	f := cmd.Flags()
	f.StringVar(&v.tenantOpts.Name, "name", "", "name of the MinIO tenant to add volumes")
	f.Int32Var(&v.tenantOpts.Servers, "servers", 0, "total number of pods to add to tenant")
	f.Int32Var(&v.tenantOpts.Volumes, "volumes", 0, "total number of volumes to add to tenant")
	f.StringVar(&v.tenantOpts.Capacity, "capacity", "", "total raw capacity to add to tenant, e.g. 16Ti")
	f.StringVarP(&v.tenantOpts.NS, "namespace", "n", helpers.DefaultNamespace, "namespace scope for this request")
	f.StringVarP(&v.tenantOpts.StorageClass, "storage-class", "s", "", "storage class to be used while PVC creation")
	f.BoolVarP(&v.output, "output", "o", false, "dry run this command and generate requisite yaml")

	return cmd
}

func (v *volumeAddCmd) validate() error {
	return v.tenantOpts.Validate()
}

// run initializes local config and installs MinIO Operator to Kubernetes cluster.
func (v *volumeAddCmd) run() error {
	// Create operator client
	client, err := helpers.GetKubeOperatorClient()
	if err != nil {
		return err
	}

	t, err := client.MinioV1().Tenants(v.tenantOpts.NS).Get(context.Background(), v.tenantOpts.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	volumesPerServer := helpers.VolumesPerServer(v.tenantOpts.Volumes, v.tenantOpts.Servers)
	capacityPerVolume, err := helpers.CapacityPerVolume(v.tenantOpts.Capacity, v.tenantOpts.Volumes)
	if err != nil {
		return err
	}

	t.Spec.Zones = append(t.Spec.Zones, resources.Zone(v.tenantOpts.Servers, volumesPerServer, *capacityPerVolume, v.tenantOpts.StorageClass))

	if !v.output {
		return addZoneToTenant(client, t)
	}

	o, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	fmt.Println(string(o))
	return nil
}

func addZoneToTenant(client *operatorv1.Clientset, t *miniov1.Tenant) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if _, err := client.MinioV1().Tenants(t.Namespace).Patch(context.Background(), t.Name, types.MergePatchType, data, metav1.PatchOptions{FieldManager: "kubectl"}); err != nil {
		return err
	}
	fmt.Printf("Adding new volumes to MinIO Tenant %s\n", t.ObjectMeta.Name)
	return nil
}
