/*
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

package deployments

import (
	"net"
	"strconv"

	miniov1 "github.com/minio/minio-operator/pkg/apis/minio.min.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Adds required MCS environment variables
func mcsEnvVars(t *miniov1.Tenant) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "MCS_MINIO_SERVER",
			Value: miniov1.Scheme + "://" + net.JoinHostPort(t.MinIOCIServiceHost(), strconv.Itoa(miniov1.MinIOPort)),
		},
	}
	if miniov1.Scheme == "https" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MCS_MINIO_SERVER_TLS_SKIP_VERIFICATION",
			Value: "on",
		})
	}
	return envVars
}

// Returns the MCS environment variables set in configuration.
func mcsSecretEnvVars(t *miniov1.Tenant) []corev1.EnvFromSource {
	envVars := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: t.Spec.MCS.MCSSecret.Name,
				},
			},
		},
	}
	return envVars
}

func mcsMetadata(t *miniov1.Tenant) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{}
	if t.HasMCSMetadata() {
		meta = *t.Spec.MCS.Metadata
	}
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	for k, v := range t.MCSPodLabels() {
		meta.Labels[k] = v
	}
	return meta
}

// mcsSelector Returns the MCS pods selector
func mcsSelector(t *miniov1.Tenant) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: t.MCSPodLabels(),
	}
}

// Builds the MCS container for a Tenant.
func mcsContainer(t *miniov1.Tenant) corev1.Container {
	args := []string{"server"}

	return corev1.Container{
		Name:  miniov1.MCSContainerName,
		Image: t.Spec.MCS.Image,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: miniov1.MCSPort,
			},
		},
		ImagePullPolicy: miniov1.DefaultImagePullPolicy,
		Args:            args,
		Env:             mcsEnvVars(t),
		EnvFrom:         mcsSecretEnvVars(t),
		Resources:       t.Spec.Resources,
	}
}

// NewForMCS creates a new Deployment for the given MinIO instance.
func NewForMCS(t *miniov1.Tenant) *appsv1.Deployment {

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       t.Namespace,
			Name:            t.MCSDeploymentName(),
			OwnerReferences: t.OwnerRef(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &t.Spec.MCS.Replicas,
			// MCS is always matched via Tenant Name + mcs prefix
			Selector: mcsSelector(t),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: mcsMetadata(t),
				Spec: corev1.PodSpec{
					Containers:    []corev1.Container{mcsContainer(t)},
					RestartPolicy: miniov1.MCSRestartPolicy,
				},
			},
		},
	}

	return d
}
