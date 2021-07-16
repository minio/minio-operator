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
	"crypto/sha1"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Adds required Console environment variables
func consoleEnvVars(t *miniov2.Tenant) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "CONSOLE_MINIO_SERVER",
			Value: t.MinIOServerEndpoint(),
		},
	}
	if t.HasLogEnabled() {
		envVars = append(envVars, corev1.EnvVar{
			Name: miniov2.LogQueryTokenKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: t.LogSecretName(),
					},
					Key: miniov2.LogQueryTokenKey,
				},
			},
		})
		url := fmt.Sprintf("http://%s:%d", t.LogSearchAPIServiceName(), miniov2.LogSearchAPIPort)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CONSOLE_LOG_QUERY_URL",
			Value: url,
		})
	}
	if t.HasPrometheusEnabled() {
		url := fmt.Sprintf("http://%s:%d", t.PrometheusHLServiceName(), miniov2.PrometheusAPIPort)
		envVars = append(envVars, corev1.EnvVar{
			Name:  miniov2.ConsolePrometheusURL,
			Value: url,
		})
	}
	// handle old versions
	if isOldConsoleVersion(t.Spec.Console.Image) && t.TLS() {
		var caCerts []string
		if t.ExternalCert() {
			for index := range t.Spec.ExternalCertSecret {
				caCerts = append(caCerts, fmt.Sprintf("%s/CAs/minio-hostname-%d.crt", miniov2.ConsoleConfigMountPath, index))
			}
		} else {
			caCerts = append(caCerts, fmt.Sprintf("%s/CAs/minio.crt", miniov2.ConsoleConfigMountPath))
		}
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CONSOLE_MINIO_SERVER_TLS_ROOT_CAS",
			Value: strings.Join(caCerts, ","),
		})
	}
	// if the PBKDF salt is not present, hash tenant name and use it as that
	// CONSOLE_PBKDF_PASSPHRASE=SECRET
	// CONSOLE_PBKDF_SALT=SECRET
	pbkdfIsSet := false
	for _, e := range t.Spec.Console.Env {
		if e.Name == "CONSOLE_PBKDF_PASSPHRASE" || e.Name == "CONSOLE_PBKDF_SALT" {
			pbkdfIsSet = true
		}
	}
	if !pbkdfIsSet {
		h := sha1.New()
		h.Write([]byte(fmt.Sprintf("%s-%s", t.Namespace, t.Name)))
		tmpHash := fmt.Sprintf("%x", h.Sum(nil))
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CONSOLE_PBKDF_PASSPHRASE",
			Value: tmpHash,
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CONSOLE_PBKDF_SALT",
			Value: tmpHash,
		})
	}

	// Add all the environment variables in the Tenant.Spec.Console CRD
	envVars = append(envVars, t.Spec.Console.Env...)
	return envVars
}

// Returns the Console environment variables set in configuration.
func consoleSecretEnvVars(t *miniov2.Tenant) []corev1.EnvFromSource {
	envVars := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: t.Spec.Console.ConsoleSecret.Name,
				},
			},
		},
	}
	return envVars
}

func consoleMetadata(t *miniov2.Tenant) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{}
	meta.Labels = t.Spec.Console.Labels
	meta.Annotations = t.Spec.Console.Annotations

	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	for k, v := range t.ConsolePodLabels() {
		meta.Labels[k] = v
	}
	return meta
}

// consoleSelector Returns the Console pods selector
func consoleSelector(t *miniov2.Tenant) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: t.ConsolePodLabels(),
	}
}

// ConsoleVolumeMounts builds the volume mounts for Console container.
func ConsoleVolumeMounts(t *miniov2.Tenant, oldConsole bool) (mounts []corev1.VolumeMount) {
	volumeMount := corev1.VolumeMount{
		Name:      t.ConsoleVolMountName(),
		MountPath: miniov2.ConsoleCertPath,
	}

	if oldConsole && (t.TLS() || t.ConsoleExternalCert()) {
		volumeMount.MountPath = miniov2.ConsoleConfigMountPath
	}

	return []corev1.VolumeMount{
		volumeMount,
	}
}

// Builds the Console container for a Tenant.
func consoleContainer(t *miniov2.Tenant, oldConsole bool) corev1.Container {
	args := []string{"server"}

	// apply old args for console versions older than v0.4.4
	if oldConsole {
		if t.AutoCert() || t.ConsoleExternalCert() {
			args = append(args, "--tls-certificate=/tmp/console/server.crt", "--tls-key=/tmp/console/server.key")
		}
	} else {
		args = append(args, fmt.Sprintf("--certs-dir=%s", miniov2.ConsoleCertPath))

	}

	return corev1.Container{
		Name:  miniov2.ConsoleContainerName,
		Image: t.Spec.Console.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: miniov2.ConsolePort,
			},
			{
				Name:          "https",
				ContainerPort: miniov2.ConsoleTLSPort,
			},
		},
		ImagePullPolicy: t.Spec.Console.ImagePullPolicy,
		Args:            args,
		Env:             consoleEnvVars(t),
		EnvFrom:         consoleSecretEnvVars(t),
		Resources:       t.Spec.Console.Resources,
		VolumeMounts:    ConsoleVolumeMounts(t, oldConsole),
	}
}

// isOldConsoleVersion returns whether image tag is older than v.0.4.4 or not
// fallbacks to false if the image is invalid
func isOldConsoleVersion(image string) bool {
	re := regexp.MustCompile(`v([0-9]+)\.([0-9]+)\.([0-9]+)`)
	matches := re.FindAllSubmatch([]byte(image), -1)

	if len(matches) == 0 || len(matches[0]) < 4 {
		return false
	}

	// check mayor, minor and patch version
	if string(matches[0][1]) == "0" && string(matches[0][2]) == "4" {
		patchVersion, err := strconv.Atoi(string(matches[0][3]))
		if err != nil {
			return false
		}
		if patchVersion < 4 {
			return true
		}
	}

	return false
}

// NewConsole creates a new Deployment for the given MinIO Tenant.
func NewConsole(t *miniov2.Tenant) *appsv1.Deployment {
	var certPath = "public.crt"
	var keyPath = "private.key"

	isOldConsole := isOldConsoleVersion(t.Spec.Console.Image)
	if isOldConsole {
		certPath = "server.crt"
		keyPath = "server.key"
	}

	var podVolumeSources []corev1.VolumeProjection

	var serverCertPaths = []corev1.KeyToPath{
		{Key: "public.crt", Path: certPath},
		{Key: "private.key", Path: keyPath},
	}

	var tenantCertPath = "CAs/minio.crt"
	var tenantCertPaths = []corev1.KeyToPath{
		{Key: "public.crt", Path: tenantCertPath},
	}
	// External certificates will have priority over AutoCert generated certificates
	// In the future this may change when Console supports SNI
	if t.ConsoleExternalCert() {
		serverCertSecret := t.Spec.Console.ExternalCertSecret.Name
		// This covers both secrets of type "kubernetes.io/tls" and
		// "cert-manager.io/v1alpha2" because of same keys in both.
		if t.Spec.Console.ExternalCertSecret.Type == "kubernetes.io/tls" || t.Spec.Console.ExternalCertSecret.Type == "cert-manager.io/v1alpha2" {
			serverCertPaths = []corev1.KeyToPath{
				{Key: "tls.crt", Path: certPath},
				{Key: "tls.key", Path: keyPath},
			}
		}
		podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: serverCertSecret,
				},
				Items: serverCertPaths,
			},
		})
	} else if t.AutoCert() {
		// Console certificates generated by AutoCert
		podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: t.ConsoleTLSSecretName(),
				},
				Items: serverCertPaths,
			},
		})
	}

	// If MinIO has AutoCert enabled load the autogenerated certificate into certs/CAS/minio.crt
	if t.AutoCert() {
		// MinIO tenant certificate generated by AutoCert
		podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: t.MinIOTLSSecretName(),
				},
				Items: tenantCertPaths,
			},
		})
	}

	// If user provided additional certificates load those too into certs/CAs/minio-hostname-{n}.crt
	if t.ExternalCert() {
		// Iterate over all provided TLS certificates and store them on the list of
		// Volumes that will be mounted to the Pod using the following folder structure:
		//
		//	certs
		//		+ CAs
		//			 + minio-hostname-0.crt
		//			 + minio-hostname-1.crt
		//			 + minio-hostname-2.crt
		//
		for index, secret := range t.Spec.ExternalCertSecret {
			// This covers both secrets of type "kubernetes.io/tls" and
			// "cert-manager.io/v1alpha2" because of same keys in both.
			if secret.Type == "kubernetes.io/tls" || secret.Type == "cert-manager.io/v1alpha2" {
				tenantCertPaths = []corev1.KeyToPath{
					{Key: "tls.crt", Path: fmt.Sprintf("CAs/minio-hostname-%d.crt", index)},
				}
			} else {
				tenantCertPaths = []corev1.KeyToPath{
					{Key: "public.crt", Path: fmt.Sprintf("CAs/minio-hostname-%d.crt", index)},
				}
			}
			podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Items: tenantCertPaths,
				},
			})
		}
	}

	// Will mount into ~/.console/certs/CAs folder the user provided CA certificates.
	// This is used for Console to verify TLS connections with other applications.
	//	certs
	//		+ CAs
	//			 + ca-0.crt
	//			 + ca-1.crt
	//			 + ca-2.crt
	if t.ConsoleExternalCaCerts() {
		for index, secret := range t.Spec.Console.ExternalCaCertSecret {
			var caCertPaths []corev1.KeyToPath
			// This covers both secrets of type "kubernetes.io/tls" and
			// "cert-manager.io/v1alpha2" because of same keys in both.
			if secret.Type == "kubernetes.io/tls" {
				caCertPaths = []corev1.KeyToPath{
					{Key: "tls.crt", Path: fmt.Sprintf("CAs/ca-%d.crt", index)},
				}
			} else if secret.Type == "cert-manager.io/v1alpha2" {
				caCertPaths = []corev1.KeyToPath{
					{Key: "ca.crt", Path: fmt.Sprintf("CAs/ca-%d.crt", index)},
				}
			} else {
				caCertPaths = []corev1.KeyToPath{
					{Key: "public.crt", Path: fmt.Sprintf("CAs/ca-%d.crt", index)},
				}
			}
			podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Items: caCertPaths,
				},
			})
		}
	}

	podVolumes := []corev1.Volume{
		{
			Name: t.ConsoleVolMountName(),
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: podVolumeSources,
				},
			},
		},
	}

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       t.Namespace,
			Name:            t.ConsoleDeploymentName(),
			OwnerReferences: t.OwnerRef(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &t.Spec.Console.Replicas,
			// Console is always matched via Tenant Name + console prefix
			Selector: consoleSelector(t),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: consoleMetadata(t),
				Spec: corev1.PodSpec{
					ServiceAccountName: t.Spec.Console.ServiceAccountName,
					Containers:         []corev1.Container{consoleContainer(t, isOldConsole)},
					RestartPolicy:      miniov2.ConsoleRestartPolicy,
					Volumes:            podVolumes,
					NodeSelector:       t.Spec.Console.NodeSelector,
					Tolerations:        t.Spec.Console.Tolerations,
					SecurityContext:    t.Spec.Console.SecurityContext,
				},
			},
		},
	}

	if t.Spec.ImagePullSecret.Name != "" {
		d.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{t.Spec.ImagePullSecret}
	}

	return d
}
