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

package cluster

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_consoleDeploymentMatchesSpec(t *testing.T) {
	type args struct {
		tenant            *miniov2.Tenant
		consoleDeployment *appsv1.Deployment
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Different image",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image: "minio/console:image1",
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Replicas: intToPtr(0),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								miniov2.ConsoleTenantLabel: "tenant-a-console",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									miniov2.ConsoleTenantLabel: "tenant-a-console",
								},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								Containers: []corev1.Container{
									{
										Name:  "console",
										Image: "minio/console:image2",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "ssshh",
													},
												},
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  "CONSOLE_MINIO_SERVER",
												Value: "https://minio..svc.cluster.local:443",
											},
											{
												Name:  "x",
												Value: "y",
											},
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												ContainerPort: 9090,
											},
											{
												Name:          "https",
												ContainerPort: 9443,
											},
										},
										Args: []string{
											"server",
											"--certs-dir=/tmp/certs",
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "tenant-a-console",
												MountPath: "/tmp/certs",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "tenant-a-console",
										VolumeSource: corev1.VolumeSource{
											Projected: &corev1.ProjectedVolumeSource{
												Sources: []corev1.VolumeProjection{
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-console-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "public.crt",
																},
																{
																	Key:  "private.key",
																	Path: "private.key",
																},
															},
														},
													},
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "CAs/minio.crt",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Same image",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image:         "minio/console:image1",
							ConsoleSecret: &corev1.LocalObjectReference{Name: "ssshh"},
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Replicas: intToPtr(0),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								miniov2.ConsoleTenantLabel: "tenant-a-console",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									miniov2.ConsoleTenantLabel: "tenant-a-console",
								},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								Containers: []corev1.Container{
									{
										Name:  "console",
										Image: "minio/console:image1",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "ssshh",
													},
												},
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  "CONSOLE_MINIO_SERVER",
												Value: "https://minio..svc.cluster.local:443",
											},
											{
												Name:  "CONSOLE_PBKDF_PASSPHRASE",
												Value: "b4e56747e3d36260d90eeeb08ba02f104044b690",
											},
											{
												Name:  "CONSOLE_PBKDF_SALT",
												Value: "b4e56747e3d36260d90eeeb08ba02f104044b690",
											},
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												ContainerPort: 9090,
											},
											{
												Name:          "https",
												ContainerPort: 9443,
											},
										},
										Args: []string{
											"server",
											"--certs-dir=/tmp/certs",
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "tenant-a-console",
												MountPath: "/tmp/certs",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "tenant-a-console",
										VolumeSource: corev1.VolumeSource{
											Projected: &corev1.ProjectedVolumeSource{
												Sources: []corev1.VolumeProjection{
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-console-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "public.crt",
																},
																{
																	Key:  "private.key",
																	Path: "private.key",
																},
															},
														},
													},
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "CAs/minio.crt",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Same resources",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image:         "minio/console:image1",
							ConsoleSecret: &corev1.LocalObjectReference{Name: "ssshh"},
							Resources: corev1.ResourceRequirements{
								Limits: nil,
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("16"),
								},
							},
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Replicas: intToPtr(0),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								miniov2.ConsoleTenantLabel: "tenant-a-console",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									miniov2.ConsoleTenantLabel: "tenant-a-console",
								},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								Containers: []corev1.Container{
									{
										Resources: corev1.ResourceRequirements{
											Limits: nil,
											Requests: corev1.ResourceList{
												corev1.ResourceCPU: resource.MustParse("16"),
											},
										},
										Name:  "console",
										Image: "minio/console:image1",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "ssshh",
													},
												},
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  "CONSOLE_MINIO_SERVER",
												Value: "https://minio..svc.cluster.local:443",
											},
											{
												Name:  "CONSOLE_PBKDF_PASSPHRASE",
												Value: "b4e56747e3d36260d90eeeb08ba02f104044b690",
											},
											{
												Name:  "CONSOLE_PBKDF_SALT",
												Value: "b4e56747e3d36260d90eeeb08ba02f104044b690",
											},
											{
												Name:  "x",
												Value: "y",
											},
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												ContainerPort: 9090,
											},
											{
												Name:          "https",
												ContainerPort: 9443,
											},
										},
										Args: []string{
											"server",
											"--certs-dir=/tmp/certs",
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "tenant-a-console",
												MountPath: "/tmp/certs",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "tenant-a-console",
										VolumeSource: corev1.VolumeSource{
											Projected: &corev1.ProjectedVolumeSource{
												Sources: []corev1.VolumeProjection{
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-console-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "public.crt",
																},
																{
																	Key:  "private.key",
																	Path: "private.key",
																},
															},
														},
													},
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "CAs/minio.crt",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Resources changed",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image:         "minio/console:image1",
							ConsoleSecret: &corev1.LocalObjectReference{Name: "ssshh"},
							Resources: corev1.ResourceRequirements{
								Limits: nil,
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("8"),
								},
							},
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "minio/console:image1",
										Resources: corev1.ResourceRequirements{
											Limits: nil,
											Requests: corev1.ResourceList{
												corev1.ResourceCPU: resource.MustParse("16"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Environment changed",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image:         "minio/console:image1",
							ConsoleSecret: &corev1.LocalObjectReference{Name: "ssshh"},
							Env: []corev1.EnvVar{
								{
									Name:  "x",
									Value: "y",
								},
							},
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Replicas: intToPtr(0),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								miniov2.ConsoleTenantLabel: "tenant-a-console",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									miniov2.ConsoleTenantLabel: "tenant-a-console",
								},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								Containers: []corev1.Container{
									{
										Name:  "console",
										Image: "minio/console:image1",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "ssshh",
													},
												},
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  "CONSOLE_MINIO_SERVER",
												Value: "https://minio..svc.cluster.local:443",
											},
											{
												Name:  "x",
												Value: "z",
											},
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												ContainerPort: 9090,
											},
											{
												Name:          "https",
												ContainerPort: 9443,
											},
										},
										Args: []string{
											"server",
											"--certs-dir=/tmp/certs",
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "tenant-a-console",
												MountPath: "/tmp/certs",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "tenant-a-console",
										VolumeSource: corev1.VolumeSource{
											Projected: &corev1.ProjectedVolumeSource{
												Sources: []corev1.VolumeProjection{
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-console-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "public.crt",
																},
																{
																	Key:  "private.key",
																	Path: "private.key",
																},
															},
														},
													},
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "CAs/minio.crt",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Same Environment",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image:         "minio/console:image1",
							ConsoleSecret: &corev1.LocalObjectReference{Name: "ssshh"},
							Env: []corev1.EnvVar{
								{
									Name:  "x",
									Value: "y",
								},
							},
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Replicas: intToPtr(0),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								miniov2.ConsoleTenantLabel: "tenant-a-console",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									miniov2.ConsoleTenantLabel: "tenant-a-console",
								},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								Containers: []corev1.Container{
									{
										Name:  "console",
										Image: "minio/console:image1",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "ssshh",
													},
												},
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  "CONSOLE_MINIO_SERVER",
												Value: "https://minio..svc.cluster.local:443",
											},
											{
												Name:  "CONSOLE_PBKDF_PASSPHRASE",
												Value: "b4e56747e3d36260d90eeeb08ba02f104044b690",
											},
											{
												Name:  "CONSOLE_PBKDF_SALT",
												Value: "b4e56747e3d36260d90eeeb08ba02f104044b690",
											},
											{
												Name:  "x",
												Value: "y",
											},
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												ContainerPort: 9090,
											},
											{
												Name:          "https",
												ContainerPort: 9443,
											},
										},
										Args: []string{
											"server",
											"--certs-dir=/tmp/certs",
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "tenant-a-console",
												MountPath: "/tmp/certs",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "tenant-a-console",
										VolumeSource: corev1.VolumeSource{
											Projected: &corev1.ProjectedVolumeSource{
												Sources: []corev1.VolumeProjection{
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-console-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "public.crt",
																},
																{
																	Key:  "private.key",
																	Path: "private.key",
																},
															},
														},
													},
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "CAs/minio.crt",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Log Search Enabled Env Missing",
			args: args{
				tenant: &miniov2.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tenant-a",
					},
					Spec: miniov2.TenantSpec{
						Console: &miniov2.ConsoleConfiguration{
							Image:         "minio/console:image1",
							ConsoleSecret: &corev1.LocalObjectReference{Name: "ssshh"},
							Env: []corev1.EnvVar{
								{
									Name:  "x",
									Value: "y",
								},
							},
						},
						Log: &miniov2.LogConfig{
							Image: "minio/log-search:image1",
						},
					},
				},
				consoleDeployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{
						Replicas: intToPtr(0),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								miniov2.ConsoleTenantLabel: "tenant-a-console",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									miniov2.ConsoleTenantLabel: "tenant-a-console",
								},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								Containers: []corev1.Container{
									{
										Name:  "console",
										Image: "minio/console:image1",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "ssshh",
													},
												},
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  "CONSOLE_MINIO_SERVER",
												Value: "https://minio..svc.cluster.local:443",
											},
											{
												Name:  "x",
												Value: "y",
											},
										},
										Ports: []corev1.ContainerPort{
											{
												Name:          "http",
												ContainerPort: 9090,
											},
											{
												Name:          "https",
												ContainerPort: 9443,
											},
										},
										Args: []string{
											"server",
											"--certs-dir=/tmp/certs",
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "tenant-a-console",
												MountPath: "/tmp/certs",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "tenant-a-console",
										VolumeSource: corev1.VolumeSource{
											Projected: &corev1.ProjectedVolumeSource{
												Sources: []corev1.VolumeProjection{
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-console-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "public.crt",
																},
																{
																	Key:  "private.key",
																	Path: "private.key",
																},
															},
														},
													},
													{
														Secret: &corev1.SecretProjection{
															LocalObjectReference: corev1.LocalObjectReference{Name: "tenant-a-tls"},
															Items: []corev1.KeyToPath{
																{
																	Key:  "public.crt",
																	Path: "CAs/minio.crt",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := consoleDeploymentMatchesSpec(tt.args.tenant, tt.args.consoleDeployment)
			if (err != nil) != tt.wantErr {
				t.Errorf("consoleDeploymentMatchesSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("consoleDeploymentMatchesSpec() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func intToPtr(x int32) *int32 {
	return &x
}
