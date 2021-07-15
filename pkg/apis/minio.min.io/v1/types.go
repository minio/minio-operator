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

package v1

import (
	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:defaulter-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=tenant,singular=tenant
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.currentState"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Tenant is a specification for a MinIO resource
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Scheduler miniov2.TenantScheduler `json:"scheduler,omitempty"`
	Spec      TenantSpec              `json:"spec"`
	// Status provides details of the state of the Tenant
	// +optional
	Status miniov2.TenantStatus `json:"status"`
}

// TenantSpec is the spec for a Tenant resource
type TenantSpec struct {
	// Users defines an array of user credential secrets that will be created on MinIO during tenant provisioning.
	// +optional
	Users []*corev1.LocalObjectReference `json:"users"`
	// Definition for Cluster in given MinIO cluster
	Zones []Zone `json:"zones"`
	// Image defines the Tenant Docker image.
	// +optional
	Image string `json:"image,omitempty"`
	// ImagePullSecret defines the secret to be used for pull image from a private Docker image.
	// +optional
	ImagePullSecret corev1.LocalObjectReference `json:"imagePullSecret,omitempty"`
	// Pod Management Policy for pod created by StatefulSet
	// +optional
	PodManagementPolicy appsv1.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`
	// If provided, use this secret as the credentials for Tenant resource
	// Otherwise MinIO server creates dynamic credentials printed on MinIO server startup banner
	// +optional
	CredsSecret *corev1.LocalObjectReference `json:"credsSecret,omitempty"`
	// If provided, use these environment variables for Tenant resource
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// ExternalCertSecret allows a user to specify one or more custom TLS certificates, and private keys. This is
	// used for enabling TLS with SNI support on MinIO Pods.
	// +optional
	ExternalCertSecret []*miniov2.LocalCertificateReference `json:"externalCertSecret,omitempty"`
	// ExternalClientCertSecret allows a user to specify custom CA client certificate, and private key. This is
	// used for adding client certificates on MinIO Pods --> used for KES authentication.
	// +optional
	ExternalClientCertSecret *miniov2.LocalCertificateReference `json:"externalClientCertSecret,omitempty"`
	// ExternalCaCertSecret allows a user to provide additional CA certificates. This is
	// used for Console to verify TLS connections with other applications.
	// +optional
	ExternalCaCertSecret []*miniov2.LocalCertificateReference `json:"externalCaCertSecret,omitempty"`
	// Mount path for MinIO volume (PV). Defaults to /export
	// +optional
	Mountpath string `json:"mountPath,omitempty"`
	// Subpath inside mount path. This is the directory where MinIO stores data. Default to "" (empty)
	// +optional
	Subpath string `json:"subPath,omitempty"`
	// RequestAutoCert allows user to enable Kubernetes based TLS cert generation and signing as explained here:
	// https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/
	// +optional
	RequestAutoCert *bool `json:"requestAutoCert,omitempty"`
	// S3 related features can be disabled or enabled such as `bucketDNS` etc.
	S3 *miniov2.S3Features `json:"s3,omitempty"`
	// +optional
	// CertConfig allows users to set entries like CommonName, Organization, etc for the certificate
	// +optional
	CertConfig *miniov2.CertificateConfig `json:"certConfig,omitempty"`
	// Security Context allows user to set entries like runAsUser, privilege escalation etc.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// ConsoleConfiguration is for setting up minio/console for graphical user interface
	//+optional
	Console *miniov2.ConsoleConfiguration `json:"console,omitempty"`
	// KES is for setting up minio/kes as MinIO KMS
	//+optional
	KES *miniov2.KESConfig `json:"kes,omitempty"`
	// Log is for setting up log search
	//+optional
	Log *miniov2.LogConfig `json:"log,omitempty"`
	// Prometheus is for setting up Prometheus metrics.
	//+optional
	Prometheus *miniov2.PrometheusConfig `json:"prometheus,omitempty"`
	// ServiceAccountName is the name of the ServiceAccount to use to run pods of all MinIO
	// Pods created as a part of this Tenant.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// PriorityClassName indicates the Pod priority and hence importance of a Pod relative to other Pods.
	// This is applied to MinIO pods only.
	// Refer Kubernetes documentation for details https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// Image pull policy. One of Always, Never, IfNotPresent.
	// This is applied to MinIO pods only.
	// Refer Kubernetes documentation for details https://kubernetes.io/docs/concepts/containers/images#updating-images
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// SideCars a list of containers to run as sidecars along every MinIO Pod on every pool
	// +optional
	SideCars *miniov2.SideCars `json:"sideCars,omitempty"`
	// ExposeServices tells operator whether to expose the MinIO service and/or the Console Service
	// +optional
	ExposeServices *miniov2.ExposeServices `json:"exposeServices,omitempty"`
	// *Optional* +
	//
	// Specify a secret that contains additional environment variable configurations to be used for the MinIO pools.
	// The secret is expected to have a key named config.env containing all exported environment variables for MinIO+
	// +optional
	Configuration *corev1.LocalObjectReference `json:"configuration,omitempty"`
}

// Zone defines the spec for a MinIO Zone
type Zone struct {
	// Name of the zone
	// +optional
	Name string `json:"name,omitempty"`
	// Number of Servers in the zone
	Servers int32 `json:"servers"`
	// Number of persistent volumes that will be attached per server
	VolumesPerServer int32 `json:"volumesPerServer"`
	// VolumeClaimTemplate allows a user to specify how volumes inside a Tenant
	VolumeClaimTemplate *corev1.PersistentVolumeClaim `json:"volumeClaimTemplate"`
	// If provided, use these requests and limit for cpu/memory resource allocation
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, affinity will define the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// Tolerations allows users to set entries like effect, key, operator, value.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TenantList is a list of Tenant resources
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Tenant `json:"items"`
}
