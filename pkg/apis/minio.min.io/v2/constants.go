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

package v2

import (
	"crypto/elliptic"
	"runtime"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// MinIOCRDResourceKind is the Kind of a Cluster.
const MinIOCRDResourceKind = "Tenant"

// OperatorCRDResourceKind is the Kind of a Cluster.
const OperatorCRDResourceKind = "Operator"

// DefaultPodManagementPolicy specifies default pod management policy as expllained here
// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#pod-management-policies
const DefaultPodManagementPolicy = appsv1.ParallelPodManagement

// DefaultUpdateStrategy specifies default pod update policy as explained here
// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#update-strategies
const DefaultUpdateStrategy = "RollingUpdate"

// DefaultImagePullPolicy specifies the policy to image pulls
const DefaultImagePullPolicy = corev1.PullIfNotPresent

// CSRNameSuffix specifies the suffix added to Tenant name to create a CSR
const CSRNameSuffix = "-csr"

// MinIO Related Constants

// MinIOCertPath is the path where all MinIO certs are mounted
const MinIOCertPath = "/tmp/certs"

// TmpPath /tmp path inside the container file system
const TmpPath = "/tmp"

// OperatorLabel denotes the version of the Tenant operator
// running in the cluster.
const OperatorLabel = "v1.min.io/version"

// TenantLabel is applied to all components of a Tenant cluster
const TenantLabel = "v1.min.io/tenant"

// PoolLabel is applied to all components in a Pool of a Tenant cluster
const PoolLabel = "v1.min.io/pool"

// LogDbLabel is applied to all log db components of a Tenant cluster
const LogDbLabel = "v1.min.io/logdb"

// Revision is applied to all statefulsets
const Revision = "min.io/revision"

// MinIOPort specifies the default Tenant port number.
const MinIOPort = 9000

// MinIOPortLoadBalancerSVC specifies the default Service port number for the load balancer service.
const MinIOPortLoadBalancerSVC = 80

// MinIOTLSPortLoadBalancerSVC specifies the default Service TLS port number for the load balancer service.
const MinIOTLSPortLoadBalancerSVC = 443

// MinIOServiceHTTPPortName specifies the default Service's http port name, e.g. for automatic protocol selection in Istio
const MinIOServiceHTTPPortName = "http-minio"

// MinIOServiceHTTPSPortName specifies the default Service's https port name, e.g. for automatic protocol selection in Istio
const MinIOServiceHTTPSPortName = "https-minio"

// MinIOVolumeName specifies the default volume name for MinIO volumes
const MinIOVolumeName = "export"

// MinIOVolumeMountPath specifies the default mount path for MinIO volumes
const MinIOVolumeMountPath = "/export"

// MinIOVolumeSubPath specifies the default sub path under mount path
const MinIOVolumeSubPath = ""

// DefaultMinIOImage specifies the default MinIO Docker hub image
const DefaultMinIOImage = "minio/minio:RELEASE.2021-06-17T00-10-46Z"

// DefaultMinIOUpdateURL specifies the default MinIO URL where binaries are
// pulled from during MinIO upgrades
const DefaultMinIOUpdateURL = "https://dl.min.io/server/minio/release/" + runtime.GOOS + "-" + runtime.GOARCH + "/archive/"

// MinIOHLSvcNameSuffix specifies the suffix added to Tenant name to create a headless service
const MinIOHLSvcNameSuffix = "-hl"

// DefaultServers specifies the default MinIO replicas to use for distributed deployment if not specified explicitly by user
const DefaultServers = 1

// DefaultVolumesPerServer specifies the default number of volumes per MinIO Tenant
const DefaultVolumesPerServer = 1

// DefaultPoolName specifies the default pool name
const DefaultPoolName = "pool-0"

// Console Related Constants

// DefaultConsoleImage specifies the latest Console Docker hub image
const DefaultConsoleImage = "minio/console:v0.7.5"

// ConsoleTenantLabel is applied to the Console pods of a Tenant cluster
const ConsoleTenantLabel = "v1.min.io/console"

// ConsolePort specifies the default Console port number.
const ConsolePort = 9090

// ConsoleServicePortName specifies the default Console Service's port name.
const ConsoleServicePortName = "http-console"

// ConsoleTLSPort specifies the default Console port number for HTTPS.
const ConsoleTLSPort = 9443

// ConsoleServiceTLSPortName specifies the default Console Service's port name.
const ConsoleServiceTLSPortName = "https-console"

// ConsoleServiceNameSuffix specifies the suffix added to Tenant service name to create a service for console
const ConsoleServiceNameSuffix = "-ui"

// ConsoleName specifies the default container name for Console
const ConsoleName = "-console"

// ConsoleAdminPolicyName denotes the policy name for Console user
const ConsoleAdminPolicyName = "consoleAdmin"

// ConsoleRestartPolicy defines the default restart policy for Console Containers
const ConsoleRestartPolicy = corev1.RestartPolicyAlways

// ConsoleConfigMountPath specifies the path where Console config file and all secrets are mounted
// We keep this to /tmp so it doesn't require any special permissions
const ConsoleConfigMountPath = "/tmp/console"

// DefaultConsoleReplicas specifies the default number of Console pods to be created if not specified
const DefaultConsoleReplicas = 2

// ConsoleCertPath is the path where all Console certs are mounted
const ConsoleCertPath = "/tmp/certs"

// Prometheus related constants

// PrometheusImage specifies the container image for prometheus server
const PrometheusImage = "quay.io/prometheus/prometheus:latest"

// PrometheusSideCarImage specifies the container image for prometheus sidecar
const PrometheusSideCarImage = "alpine"

// PrometheusInitImage specifies the init container image for prometheus server
const PrometheusInitImage = "busybox"

// PrometheusInstanceLabel is applied to the prometheus server pod
const PrometheusInstanceLabel = "v1.min.io/prometheus"

// PrometheusPort specifies the default prometheus port number
const PrometheusPort = 9090

// PrometheusPortName specifies the default prometheus port's name.
const PrometheusPortName = "http-prometheus"

// PrometheusHLSvcNameSuffix specifies the suffix added to Tenant name to create
// a headless service for Prometheus.
const PrometheusHLSvcNameSuffix = "-prometheus-hl-svc"

// PrometheusServiceMonitorSecretKey is the entry in the secret
const PrometheusServiceMonitorSecretKey = "token"

// Log related constants

// DefaultLogSearchAPIImage specifies the latest logsearchapi container image
const DefaultLogSearchAPIImage = "minio/logsearchapi:v4.1.3"

// LogPgImage specifies the latest Postgres container image
const LogPgImage = "library/postgres"

// LogDBInstanceLabel is applied to the Log (Postgres server) pods
const LogDBInstanceLabel = "v1.min.io/log-pg"

// LogSearchAPIInstanceLabel is applied to the Log Search API server pods
const LogSearchAPIInstanceLabel = "v1.min.io/logsearchapi"

// LogPgPort specifies the default Log Service's port number.
const LogPgPort = 5432

// LogSearchAPIPort specifies the default Log Search API Service's port number.
const LogSearchAPIPort = 8080

// LogPgPortName specifies the default Log Service Postgres server's port name.
const LogPgPortName = "http-log-pg"

// LogSearchAPIPortName specifies the default Log Search API server's port name.
const LogSearchAPIPortName = "http-logsearchapi"

// LogHLSvcNameSuffix specifies the suffix added to Tenant name to create a headless service for Log
const LogHLSvcNameSuffix = "-log-hl-svc"

// LogPgUserKey is the k8s secret/environment variable key name referring to postgres user
const LogPgUserKey = "POSTGRES_USER"

// LogPgPassKey is the k8s secret/environment variable key name referring to postgress password
const LogPgPassKey = "POSTGRES_PASSWORD"

// LogAuditDBKey is the k8s secret/environment variable key name referring to postgress default database.
const LogAuditDBKey = "POSTGRES_DB"

// LogPgConnStr is the k8s env var key name referring to the postgres connection string; used in logsearchapi deployment
const LogPgConnStr = "LOGSEARCH_PG_CONN_STR"

// LogPgUser is the POSTGRES_USER used for Log feature
const LogPgUser = "postgres"

// LogAuditDB holds the name of the DB used to store MinIO audit events
const LogAuditDB = "minio_logs"

// LogAuditTokenKey is the k8s secret/environment variable key name referring to
// the token used to authenticate audit log ingestion from tenant's MinIO
const LogAuditTokenKey = "LOGSEARCH_AUDIT_AUTH_TOKEN"

// LogQueryTokenKey is the k8s secret/environment variable key name referring to
// the token used to perform search query on audit logs persisted.
const LogQueryTokenKey = "LOGSEARCH_QUERY_AUTH_TOKEN"

// ConsolePrometheusURL is the url to the prometheus the console should use to pull metrics from.
const ConsolePrometheusURL = "CONSOLE_PROMETHEUS_URL"

// PrometheusAPIPort specifies the default Prometheus API Service's port number.
const PrometheusAPIPort = 9090

// LogSearchDiskCapacityGB is the k8s secret/environment variable key name
// referring to disk capacity required to store tenant's audit logs
const LogSearchDiskCapacityGB = "LOGSEARCH_DISK_CAPACITY_GB"

// KES Related Constants

// DefaultKESImage specifies the latest KES Docker hub image
const DefaultKESImage = "minio/kes:v0.14.0"

// KESInstanceLabel is applied to the KES pods of a Tenant cluster
const KESInstanceLabel = "v1.min.io/kes"

// KESPort specifies the default KES Service's port number.
const KESPort = 7373

// KESServicePortName specifies the default KES Service's port name.
const KESServicePortName = "http-kes"

// KESMinIOKey is the name of key that KES creates on the KMS backend
const KESMinIOKey = "my-minio-key"

// KESJobRestartPolicy specifies the restart policy for the job created for key creation
const KESJobRestartPolicy = corev1.RestartPolicyOnFailure

// KESHLSvcNameSuffix specifies the suffix added to Tenant name to create a headless service for KES
const KESHLSvcNameSuffix = "-kes-hl-svc"

// KESName specifies the default container name for KES
const KESName = "-kes"

// KESConfigMountPath specifies the path where KES config file and all secrets are mounted
// We keep this to /tmp so it doesn't require any special permissions
const KESConfigMountPath = "/tmp/kes"

// DefaultKESReplicas specifies the default number of KES pods to be created if not specified
const DefaultKESReplicas = 2

// Auto TLS related constants

// DefaultEllipticCurve specifies the default elliptic curve to be used for key generation
var DefaultEllipticCurve = elliptic.P256()

// DefaultOrgName specifies the default Org name to be used in automatic certificate generation
var DefaultOrgName = []string{"system:nodes"}

// DefaultQueryInterval specifies the interval between each query for CSR Status
var DefaultQueryInterval = time.Second * 5

// DefaultQueryTimeout specifies the timeout for query for CSR Status
var DefaultQueryTimeout = time.Minute * 20

// TLSSecretSuffix is the suffix applied to Tenant name to create the TLS secret
var TLSSecretSuffix = "-tls"

// Cluster Domain
const clusterDomain = "CLUSTER_DOMAIN"

// StatefulSetPrefix used by statefulsets
const StatefulSetPrefix = "ss"

// StatefulSetLegacyPrefix by old operators
const StatefulSetLegacyPrefix = "zone"

// MinIOPrometheusPathCluster is the path where MinIO tenant exposes cluster Prometheus metrics
const MinIOPrometheusPathCluster = "/minio/v2/metrics/cluster"

// MinIOPrometheusPathNode is the path where MinIO tenant exposes node Prometheus metrics
const MinIOPrometheusPathNode = "/minio/v2/metrics/node"

// MinIOPrometheusScrapeInterval defines how frequently to scrape targets.
const MinIOPrometheusScrapeInterval = 30 * time.Second

// MinIOPrometheusScrapeTimeout defines the timeout for scrape requests
const MinIOPrometheusScrapeTimeout = 2 * time.Second

const tenantMinIOImageEnv = "TENANT_MINIO_IMAGE"

const tenantConsoleImageEnv = "TENANT_CONSOLE_IMAGE"

const tenantKesImageEnv = "TENANT_KES_IMAGE"

const monitoringIntervalEnv = "MONITORING_INTERVAL"

// DefaultMonitoringInterval is how often we run monitoring on tenants
const DefaultMonitoringInterval = 3
