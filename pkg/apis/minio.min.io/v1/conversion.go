// This file is part of MinIO Console Server
// Copyright (c) 2020 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package v1

import (
	v2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// +kubebuilder:docs-gen:collapse=Imports

/*
Our "spoke" versions need to implement the
[`Convertible`](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/conversion?tab=doc#Convertible)
interface.  Namely, they'll need `ConvertTo` and `ConvertFrom` methods to convert to/from
the hub version.
*/

// ConvertTo converts this v1.Tenant to the Hub version (v2).
func (src *Tenant) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2.Tenant)

	var pools []v2.Pool

	for _, zone := range src.Spec.Zones {
		pools = append(pools, v2.Pool{
			Name:                zone.Name,
			Servers:             zone.Servers,
			VolumesPerServer:    zone.VolumesPerServer,
			VolumeClaimTemplate: zone.VolumeClaimTemplate,
			Resources:           zone.Resources,
			NodeSelector:        zone.NodeSelector,
			Affinity:            zone.Affinity,
			Tolerations:         zone.Tolerations,
		})
	}

	dst.Spec.Pools = pools

	/*
		The rest of the conversion is pretty rote.
	*/
	dst.Kind = "Tenant"
	dst.APIVersion = "minio.min.io/v2"

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.Users = src.Spec.Users
	dst.Spec.Image = src.Spec.Image
	dst.Spec.ImagePullSecret = src.Spec.ImagePullSecret
	dst.Spec.PodManagementPolicy = src.Spec.PodManagementPolicy
	dst.Spec.CredsSecret = src.Spec.CredsSecret
	dst.Spec.Env = src.Spec.Env
	dst.Spec.Configuration = src.Spec.Configuration
	dst.Spec.ExternalCertSecret = src.Spec.ExternalCertSecret
	dst.Spec.ExternalCaCertSecret = src.Spec.ExternalCaCertSecret
	dst.Spec.ExternalClientCertSecret = src.Spec.ExternalClientCertSecret
	dst.Spec.Mountpath = src.Spec.Mountpath
	dst.Spec.Subpath = src.Spec.Subpath
	dst.Spec.RequestAutoCert = src.Spec.RequestAutoCert
	dst.Spec.S3 = src.Spec.S3
	dst.Spec.CertConfig = src.Spec.CertConfig
	dst.Spec.Console = src.Spec.Console
	dst.Spec.KES = src.Spec.KES
	dst.Spec.Log = src.Spec.Log
	dst.Spec.Prometheus = src.Spec.Prometheus
	dst.Spec.ServiceAccountName = src.Spec.ServiceAccountName
	dst.Spec.PriorityClassName = src.Spec.PriorityClassName
	dst.Spec.ImagePullPolicy = src.Spec.ImagePullPolicy
	dst.Spec.SideCars = src.Spec.SideCars
	dst.Spec.ExposeServices = src.Spec.ExposeServices
	// Apply the securityContext to all the Pools
	for _, p := range dst.Spec.Pools {
		p.SecurityContext = src.Spec.SecurityContext
	}

	// Status
	dst.Status.AvailableReplicas = src.Status.AvailableReplicas
	dst.Status.CurrentState = src.Status.CurrentState
	dst.Status.Certificates = src.Status.Certificates
	dst.Status.SyncVersion = src.Status.SyncVersion

	// +kubebuilder:docs-gen:collapse=rote conversion
	return nil
}

// ConvertFrom converts from the Hub version (v2) to this version.
func (dst *Tenant) ConvertFrom(srcRaw conversion.Hub) error { //nolint
	src := srcRaw.(*v2.Tenant)

	var zones []Zone

	for _, pool := range src.Spec.Pools {
		zones = append(zones, Zone{
			Name:                pool.Name,
			Servers:             pool.Servers,
			VolumesPerServer:    pool.VolumesPerServer,
			VolumeClaimTemplate: pool.VolumeClaimTemplate,
			Resources:           pool.Resources,
			NodeSelector:        pool.NodeSelector,
			Affinity:            pool.Affinity,
			Tolerations:         pool.Tolerations,
		})
	}

	dst.Spec.Zones = zones

	/*
		The rest of the conversion is pretty rote.
	*/

	dst.Kind = "Tenant"
	dst.APIVersion = "minio.min.io/v1"

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.Users = src.Spec.Users
	dst.Spec.Image = src.Spec.Image
	dst.Spec.ImagePullSecret = src.Spec.ImagePullSecret
	dst.Spec.PodManagementPolicy = src.Spec.PodManagementPolicy
	dst.Spec.CredsSecret = src.Spec.CredsSecret
	dst.Spec.Env = src.Spec.Env
	dst.Spec.ExternalCertSecret = src.Spec.ExternalCertSecret
	dst.Spec.ExternalCaCertSecret = src.Spec.ExternalCaCertSecret
	dst.Spec.ExternalClientCertSecret = src.Spec.ExternalClientCertSecret
	dst.Spec.Mountpath = src.Spec.Mountpath
	dst.Spec.Subpath = src.Spec.Subpath
	dst.Spec.RequestAutoCert = src.Spec.RequestAutoCert
	dst.Spec.S3 = src.Spec.S3
	dst.Spec.CertConfig = src.Spec.CertConfig
	dst.Spec.Console = src.Spec.Console
	dst.Spec.KES = src.Spec.KES
	dst.Spec.Log = src.Spec.Log
	dst.Spec.Prometheus = src.Spec.Prometheus
	dst.Spec.ServiceAccountName = src.Spec.ServiceAccountName
	dst.Spec.PriorityClassName = src.Spec.PriorityClassName
	dst.Spec.ImagePullPolicy = src.Spec.ImagePullPolicy
	dst.Spec.SideCars = src.Spec.SideCars
	dst.Spec.ExposeServices = src.Spec.ExposeServices
	if len(src.Spec.Pools) > 0 {
		dst.Spec.SecurityContext = src.Spec.Pools[0].SecurityContext
	} else {
		dst.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}

	// Status
	dst.Status.AvailableReplicas = src.Status.AvailableReplicas
	dst.Status.CurrentState = src.Status.CurrentState
	dst.Status.Certificates = src.Status.Certificates
	dst.Status.SyncVersion = src.Status.SyncVersion

	// +kubebuilder:docs-gen:collapse=rote conversion
	return nil
}
