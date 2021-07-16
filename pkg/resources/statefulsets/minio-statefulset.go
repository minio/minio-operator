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

package statefulsets

import (
	"fmt"
	"strconv"
	"strings"

	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Returns the MinIO environment variables set in configuration.
// If a user specifies a secret in the spec (for MinIO credentials) we use
// that to set MINIO_ROOT_USER & MINIO_ROOT_PASSWORD.
func minioEnvironmentVars(t *miniov2.Tenant, wsSecret *v1.Secret, hostsTemplate string, opVersion string) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	// Add all the environment variables
	envVars = append(envVars, t.Spec.Env...)

	// Enable `mc admin update` style updates to MinIO binaries
	// within the container, only operator is supposed to perform
	// these operations.
	envVars = append(envVars,
		corev1.EnvVar{
			Name:  "MINIO_UPDATE",
			Value: "on",
		}, corev1.EnvVar{
			Name:  "MINIO_UPDATE_MINISIGN_PUBKEY",
			Value: "RWTx5Zr1tiHQLwG9keckT0c45M3AGeHD6IvimQHpyRywVWGbP1aVSGav",
		}, corev1.EnvVar{
			Name: miniov2.WebhookMinIOArgs,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: miniov2.WebhookSecret,
					},
					Key: miniov2.WebhookMinIOArgs,
				},
			},
		}, corev1.EnvVar{
			// Add a fallback in-case operator is down.
			Name:  "MINIO_ENDPOINTS",
			Value: strings.Join(GetContainerArgs(t, hostsTemplate), " "),
		}, corev1.EnvVar{
			Name:  "MINIO_OPERATOR_VERSION",
			Value: opVersion,
		})

	// Enable Bucket DNS only if asked for by default turned off
	if t.S3BucketDNS() {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MINIO_DOMAIN",
			Value: t.MinIOBucketBaseDomain(),
		}, corev1.EnvVar{
			Name: miniov2.WebhookMinIOBucket,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: miniov2.WebhookSecret,
					},
					Key: miniov2.WebhookMinIOArgs,
				},
			},
		})
	}

	// Add env variables from credentials secret, if no secret provided, dont use
	// env vars. MinIO server automatically creates default credentials
	if t.HasCredsSecret() {
		secretName := t.Spec.CredsSecret.Name
		envVars = append(envVars, corev1.EnvVar{
			Name: "MINIO_ROOT_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: "accesskey",
				},
			},
		}, corev1.EnvVar{
			Name: "MINIO_ROOT_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: "secretkey",
				},
			},
		})
	}

	if t.HasKESEnabled() {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MINIO_KMS_KES_ENDPOINT",
			Value: t.KESServiceEndpoint(),
		}, corev1.EnvVar{
			Name:  "MINIO_KMS_KES_CERT_FILE",
			Value: miniov2.MinIOCertPath + "/client.crt",
		}, corev1.EnvVar{
			Name:  "MINIO_KMS_KES_KEY_FILE",
			Value: miniov2.MinIOCertPath + "/client.key",
		}, corev1.EnvVar{
			Name:  "MINIO_KMS_KES_CA_PATH",
			Value: miniov2.MinIOCertPath + "/CAs/kes.crt",
		}, corev1.EnvVar{
			Name:  "MINIO_KMS_KES_KEY_NAME",
			Value: t.Spec.KES.KeyName,
		})
	}

	if t.HasConfigurationSecret() {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MINIO_CONFIG_ENV_FILE",
			Value: miniov2.TmpPath + "/config.env",
		})
	}

	// Return environment variables
	return envVars
}

// PodMetadata Returns the MinIO pods metadata set in configuration.
// If a user specifies metadata in the spec we return that metadata.
func PodMetadata(t *miniov2.Tenant, pool *miniov2.Pool, opVersion string) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{}
	// Copy Labels and Annotations from Tenant
	meta.Labels = t.ObjectMeta.Labels
	meta.Annotations = t.ObjectMeta.Annotations

	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}

	meta.Annotations[miniov2.Revision] = fmt.Sprintf("%d", t.Status.Revision)

	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	// Add the additional label used by StatefulSet spec selector
	for k, v := range t.MinIOPodLabels() {
		meta.Labels[k] = v
	}
	// Add information labels, such as which pool we are building this pod about
	meta.Labels[miniov2.PoolLabel] = pool.Name
	meta.Labels[miniov2.OperatorLabel] = opVersion
	return meta
}

// ContainerMatchLabels Returns the labels that match the Pods in the statefulset
func ContainerMatchLabels(t *miniov2.Tenant, pool *miniov2.Pool) *metav1.LabelSelector {
	labels := t.MinIOPodLabels()
	// Add pool information so it's passed down to the underlying PVCs
	labels[miniov2.PoolLabel] = pool.Name
	return &metav1.LabelSelector{
		MatchLabels: labels,
	}
}

// Builds the volume mounts for MinIO container.
func volumeMounts(t *miniov2.Tenant, pool *miniov2.Pool) (mounts []corev1.VolumeMount) {
	// This is the case where user didn't provide a pool and we deploy a EmptyDir based
	// single node single drive (FS) MinIO deployment
	name := miniov2.MinIOVolumeName
	if pool.VolumeClaimTemplate != nil {
		name = pool.VolumeClaimTemplate.Name
	}

	if pool.VolumesPerServer == 1 {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      name + strconv.Itoa(0),
			MountPath: t.Spec.Mountpath,
		})
	} else {
		for i := 0; i < int(pool.VolumesPerServer); i++ {
			mounts = append(mounts, corev1.VolumeMount{
				Name:      name + strconv.Itoa(i),
				MountPath: t.Spec.Mountpath + strconv.Itoa(i),
			})
		}
	}

	// CertPath (/tmp/certs) will always be mounted even if the tenant doesnt have any TLS certificate
	// operator will still mount the operator public cert under /tmp/certs/CAs/operator.crt
	mounts = append(mounts, corev1.VolumeMount{
		Name:      t.MinIOTLSSecretName(),
		MountPath: miniov2.MinIOCertPath,
	})

	if t.HasConfigurationSecret() {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "configuration",
			MountPath: miniov2.TmpPath,
		})
	}

	return mounts
}

// Builds the MinIO container for a Tenant.
func poolMinioServerContainer(t *miniov2.Tenant, wsSecret *v1.Secret, pool *miniov2.Pool, hostsTemplate string, opVersion string) corev1.Container {
	args := []string{"server", "--certs-dir", miniov2.MinIOCertPath}
	if t.Spec.Logging != nil {
		// If logging is specified, expect users to
		// provide the right set of settings to toggle
		// various flags.
		if t.Spec.Logging.JSON {
			args = append(args, "--json")
		}
		if t.Spec.Logging.Anonymous {
			args = append(args, "--anonymous")
		}
		if t.Spec.Logging.Quiet {
			args = append(args, "--quiet")
		}
	}

	return corev1.Container{
		Name:  miniov2.MinIOServerName,
		Image: t.Spec.Image,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: miniov2.MinIOPort,
			},
		},
		ImagePullPolicy: t.Spec.ImagePullPolicy,
		VolumeMounts:    volumeMounts(t, pool),
		Args:            args,
		Env:             minioEnvironmentVars(t, wsSecret, hostsTemplate, opVersion),
		Resources:       pool.Resources,
	}
}

// GetContainerArgs returns the arguments that the MinIO container receives
func GetContainerArgs(t *miniov2.Tenant, hostsTemplate string) []string {
	var args []string
	if len(t.Spec.Pools) == 1 && t.Spec.Pools[0].Servers == 1 {
		// to run in standalone mode we must pass the path
		args = append(args, t.VolumePathForPool(&t.Spec.Pools[0]))
	} else {
		for index, endpoint := range t.MinIOEndpoints(hostsTemplate) {
			args = append(args, fmt.Sprintf("%s%s", endpoint, t.VolumePathForPool(&t.Spec.Pools[index])))
		}
	}
	return args
}

// Builds the tolerations for a Pool.
func minioPoolTolerations(z *miniov2.Pool) []corev1.Toleration {
	var tolerations []corev1.Toleration
	return append(tolerations, z.Tolerations...)
}

// Builds the security context for a Pool
func minioSecurityContext(pool *miniov2.Pool) *corev1.PodSecurityContext {
	var securityContext = corev1.PodSecurityContext{}
	if pool != nil && pool.SecurityContext != nil {
		securityContext = *pool.SecurityContext
	}
	return &securityContext
}

// NewPool creates a new StatefulSet for the given Cluster.
func NewPool(t *miniov2.Tenant, wsSecret *v1.Secret, pool *miniov2.Pool, serviceName string, hostsTemplate, operatorVersion string) *appsv1.StatefulSet {
	var podVolumes []corev1.Volume
	var replicas = pool.Servers
	var podVolumeSources []corev1.VolumeProjection

	var serverCertPaths = []corev1.KeyToPath{
		{Key: "public.crt", Path: "public.crt"},
		{Key: "private.key", Path: "private.key"},
		{Key: "public.crt", Path: "CAs/public.crt"},
	}

	var clientCertSecret string
	var clientCertPaths = []corev1.KeyToPath{
		{Key: "public.crt", Path: "client.crt"},
		{Key: "private.key", Path: "client.key"},
	}
	var kesCertSecret string
	var KESCertPath = []corev1.KeyToPath{
		{Key: "public.crt", Path: "CAs/kes.crt"},
	}

	// AutoCert certificates will be used for internal communication if requested
	if t.AutoCert() {
		podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: t.MinIOTLSSecretName(),
				},
				Items: serverCertPaths,
			},
		})
	}

	if t.ExternalCert() {
		// MinIO requires to have at least 1 certificate keyPair under the `certs` folder, by default
		// we will take the first secret as the main certificate keyPair if AutoCert is not enabled
		//
		//	certs
		//		+ public.crt
		//		+ private.key
		if len(t.Spec.ExternalCertSecret) > 0 && !t.AutoCert() {
			secret := t.Spec.ExternalCertSecret[0]
			// if secret type is `cert-manager.io/v1alpha2` user is providing his own CA in the ca.crt file, if not by default
			// we are going to use public.crt as the ca certificate
			if secret.Type == "kubernetes.io/tls" {
				serverCertPaths = []corev1.KeyToPath{
					{Key: "tls.crt", Path: "public.crt"},
					{Key: "tls.key", Path: "private.key"},
					{Key: "tls.crt", Path: "CAs/public.crt"},
				}
			} else if secret.Type == "cert-manager.io/v1alpha2" {
				serverCertPaths = []corev1.KeyToPath{
					{Key: "public.crt", Path: "public.crt"},
					{Key: "private.key", Path: "private.key"},
					{Key: "ca.crt", Path: "CAs/public.crt"},
				}
			}
			podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Items: serverCertPaths,
				},
			})
		}
		// Multiple certificates will be mounted using the following folder structure:
		//
		//	certs
		//		+ public.crt
		//		+ private.key
		//		|
		//		+ hostname-0
		//		|			+ public.crt
		//		|			+ private.key
		//		+ hostname-1
		//		|			+ public.crt
		//		|			+ private.key
		//		+ hostname-2
		//		|			+ public.crt
		//		|			+ private.key
		//		+ CAs
		//			 + hostname-0.crt
		//			 + hostname-1.crt
		//			 + hostname-2.crt
		//			 + public.crt
		//
		// Iterate over all provided TLS certificates (including the one we already mounted) and store them on the list of
		// Volumes that will be mounted to the Pod
		for index, secret := range t.Spec.ExternalCertSecret {
			var serverCertPaths []corev1.KeyToPath
			if secret.Type == "kubernetes.io/tls" {
				serverCertPaths = []corev1.KeyToPath{
					{Key: "tls.crt", Path: fmt.Sprintf("hostname-%d/public.crt", index)},
					{Key: "tls.key", Path: fmt.Sprintf("hostname-%d/private.key", index)},
					{Key: "tls.crt", Path: fmt.Sprintf("CAs/hostname-%d.crt", index)},
				}
			} else if secret.Type == "cert-manager.io/v1alpha2" {
				serverCertPaths = []corev1.KeyToPath{
					{Key: "tls.crt", Path: fmt.Sprintf("hostname-%d/public.crt", index)},
					{Key: "tls.key", Path: fmt.Sprintf("hostname-%d/private.key", index)},
					{Key: "ca.crt", Path: fmt.Sprintf("CAs/hostname-%d.crt", index)},
				}
			} else {
				serverCertPaths = []corev1.KeyToPath{
					{Key: "public.crt", Path: fmt.Sprintf("hostname-%d/public.crt", index)},
					{Key: "private.key", Path: fmt.Sprintf("hostname-%d/private.key", index)},
					{Key: "public.crt", Path: fmt.Sprintf("CAs/hostname-%d.crt", index)},
				}
			}
			podVolumeSources = append(podVolumeSources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Items: serverCertPaths,
				},
			})
		}
	}

	// Will mount into ~/.minio/certs/CAs folder the user provided CA certificates.
	// This is used for MinIO to verify TLS connections with other applications.
	//	certs
	//		+ CAs
	//			 + ca-0.crt
	//			 + ca-1.crt
	//			 + ca-2.crt
	if t.ExternalCaCerts() {
		for index, secret := range t.Spec.ExternalCaCertSecret {
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

	// Mount Operator TLS certificate to MinIO ~/cert/CAs
	operatorTLSSecretName := "operator-tls"
	podVolumeSources = append(podVolumeSources, []corev1.VolumeProjection{
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: operatorTLSSecretName,
				},
				Items: []corev1.KeyToPath{
					{Key: "public.crt", Path: "CAs/operator.crt"},
				},
			},
		},
	}...)

	// If KES is enable mount TLS certificate secrets
	if t.HasKESEnabled() {
		// External Client certificates will have priority over AutoCert generated certificates
		if t.ExternalClientCert() {
			clientCertSecret = t.Spec.ExternalClientCertSecret.Name
			// This covers both secrets of type "kubernetes.io/tls" and
			// "cert-manager.io/v1alpha2" because of same keys in both.
			if t.Spec.ExternalClientCertSecret.Type == "kubernetes.io/tls" || t.Spec.ExternalClientCertSecret.Type == "cert-manager.io/v1alpha2" {
				clientCertPaths = []corev1.KeyToPath{
					{Key: "tls.crt", Path: "client.crt"},
					{Key: "tls.key", Path: "client.key"},
				}
			}
		} else {
			clientCertSecret = t.MinIOClientTLSSecretName()
		}

		// KES External certificates will have priority over AutoCert generated certificates
		if t.KESExternalCert() {
			kesCertSecret = t.Spec.KES.ExternalCertSecret.Name
			// This covers both secrets of type "kubernetes.io/tls" and
			// "cert-manager.io/v1alpha2" because of same keys in both.
			if t.Spec.KES.ExternalCertSecret.Type == "kubernetes.io/tls" || t.Spec.KES.ExternalCertSecret.Type == "cert-manager.io/v1alpha2" {
				KESCertPath = []corev1.KeyToPath{
					{Key: "tls.crt", Path: "CAs/kes.crt"},
				}
			}
		} else {
			kesCertSecret = t.KESTLSSecretName()
		}

		podVolumeSources = append(podVolumeSources, []corev1.VolumeProjection{
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: clientCertSecret,
					},
					Items: clientCertPaths,
				},
			},
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: kesCertSecret,
					},
					Items: KESCertPath,
				},
			},
		}...)
	}

	if len(podVolumeSources) > 0 {
		podVolumes = append(podVolumes, corev1.Volume{
			Name: t.MinIOTLSSecretName(),
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: podVolumeSources,
				},
			},
		})
	}

	if t.HasConfigurationSecret() {
		podVolumes = append(podVolumes, corev1.Volume{
			Name: "configuration",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: t.Spec.Configuration.Name,
								},
							},
						},
					},
				},
			},
		})
	}

	ssMeta := metav1.ObjectMeta{
		Namespace: t.Namespace,
		Name:      t.PoolStatefulsetName(pool),
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(t, schema.GroupVersionKind{
				Group:   miniov2.SchemeGroupVersion.Group,
				Version: miniov2.SchemeGroupVersion.Version,
				Kind:    miniov2.MinIOCRDResourceKind,
			}),
		},
	}
	// Copy labels and annotations from the Tenant.Spec.Metadata
	ssMeta.Labels = t.ObjectMeta.Labels
	ssMeta.Annotations = t.ObjectMeta.Annotations

	if ssMeta.Labels == nil {
		ssMeta.Labels = make(map[string]string)
	}

	// Add information labels, such as which pool we are building this pod about
	ssMeta.Labels[miniov2.PoolLabel] = pool.Name
	ssMeta.Labels[miniov2.TenantLabel] = t.Name

	containers := []corev1.Container{
		poolMinioServerContainer(t, wsSecret, pool, hostsTemplate, operatorVersion),
	}

	// attach any sidecar containers and volumes
	if t.Spec.SideCars != nil && len(t.Spec.SideCars.Containers) > 0 {
		containers = append(containers, t.Spec.SideCars.Containers...)
		podVolumes = append(podVolumes, t.Spec.SideCars.Volumes...)
	}

	ss := &appsv1.StatefulSet{
		ObjectMeta: ssMeta,
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: miniov2.DefaultUpdateStrategy,
			},
			PodManagementPolicy: t.Spec.PodManagementPolicy,
			Selector:            ContainerMatchLabels(t, pool),
			ServiceName:         serviceName,
			Replicas:            &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: PodMetadata(t, pool, operatorVersion),
				Spec: corev1.PodSpec{
					Containers:         containers,
					Volumes:            podVolumes,
					RestartPolicy:      corev1.RestartPolicyAlways,
					Affinity:           pool.Affinity,
					NodeSelector:       pool.NodeSelector,
					SchedulerName:      t.Scheduler.Name,
					Tolerations:        minioPoolTolerations(pool),
					SecurityContext:    minioSecurityContext(pool),
					ServiceAccountName: t.Spec.ServiceAccountName,
					PriorityClassName:  t.Spec.PriorityClassName,
				},
			},
		},
	}

	// Address issue https://github.com/kubernetes/kubernetes/issues/85332
	if t.Spec.ImagePullSecret.Name != "" {
		ss.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{t.Spec.ImagePullSecret}
	}

	if pool.VolumeClaimTemplate != nil {
		pvClaim := *pool.VolumeClaimTemplate
		name := pvClaim.Name
		for i := 0; i < int(pool.VolumesPerServer); i++ {
			pvClaim.Name = name + strconv.Itoa(i)
			ss.Spec.VolumeClaimTemplates = append(ss.Spec.VolumeClaimTemplates, pvClaim)
		}
	}
	// attach any sidecar containers and volumes
	if t.Spec.SideCars != nil && len(t.Spec.SideCars.VolumeClaimTemplates) > 0 {
		ss.Spec.VolumeClaimTemplates = append(ss.Spec.VolumeClaimTemplates, t.Spec.SideCars.VolumeClaimTemplates...)
	}
	return ss
}
