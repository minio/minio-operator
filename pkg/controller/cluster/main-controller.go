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

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"

	"github.com/minio/madmin-go"

	"golang.org/x/time/rate"

	"github.com/docker/cli/cli/config/configfile"

	"k8s.io/klog/v2"

	// Workaround for auth import issues refer https://github.com/minio/operator/issues/283
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	prominformers "github.com/prometheus-operator/prometheus-operator/pkg/client/informers/externalversions/monitoring/v1"
	promlisters "github.com/prometheus-operator/prometheus-operator/pkg/client/listers/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	batchinformers "k8s.io/client-go/informers/batch/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	certapi "k8s.io/client-go/kubernetes/typed/certificates/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	promclientset "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	queue "k8s.io/client-go/util/workqueue"

	"github.com/dgrijalva/jwt-go"
	jwtreq "github.com/dgrijalva/jwt-go/request"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	clientset "github.com/minio/operator/pkg/client/clientset/versioned"
	minioscheme "github.com/minio/operator/pkg/client/clientset/versioned/scheme"
	informers "github.com/minio/operator/pkg/client/informers/externalversions/minio.min.io/v2"
	listers "github.com/minio/operator/pkg/client/listers/minio.min.io/v2"
	"github.com/minio/operator/pkg/resources/configmaps"
	"github.com/minio/operator/pkg/resources/deployments"
	"github.com/minio/operator/pkg/resources/secrets"
	"github.com/minio/operator/pkg/resources/servicemonitor"
	"github.com/minio/operator/pkg/resources/services"
	"github.com/minio/operator/pkg/resources/statefulsets"
)

const (
	controllerAgentName = "minio-operator"
	// SuccessSynced is used as part of the Event 'reason' when a Tenant is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Tenant fails
	// to sync due to a StatefulSet of the same name already existing.
	ErrResourceExists = "ErrResourceExists"
	// MessageResourceExists is the message used for Events when a Tenant
	// fails to sync due to a StatefulSet already existing
	MessageResourceExists = "Resource %q already exists and is not managed by MinIO Operator"
	// MessageResourceSynced is the message used for an Event fired when a Tenant
	// is synced successfully
	MessageResourceSynced = "Tenant synced successfully"
)

// Standard Status messages for Tenant
const (
	StatusInitialized                          = "Initialized"
	StatusProvisioningCIService                = "Provisioning MinIO Cluster IP Service"
	StatusProvisioningHLService                = "Provisioning MinIO Headless Service"
	StatusProvisioningStatefulSet              = "Provisioning MinIO Statefulset"
	StatusProvisioningConsoleDeployment        = "Provisioning Console Deployment"
	StatusProvisioningKESStatefulSet           = "Provisioning KES StatefulSet"
	StatusProvisioningLogPGStatefulSet         = "Provisioning Postgres server for Log Search feature"
	StatusProvisioningLogSearchAPIDeployment   = "Provisioning Log Search API server for Log Search feature"
	StatusProvisioningPrometheusStatefulSet    = "Provisioning Prometheus server for Prometheus metrics feature"
	StatusProvisioningPrometheusServiceMonitor = "Provisioning Prometheus service monitor for Prometheus metrics feature"
	StatusWaitingForReadyState                 = "Waiting for Pods to be ready"
	StatusWaitingForLogSearchReadyState        = "Waiting for Log Search Pods to be ready"
	StatusWaitingMinIOCert                     = "Waiting for MinIO TLS Certificate"
	StatusWaitingMinIOClientCert               = "Waiting for MinIO TLS Client Certificate"
	StatusWaitingKESCert                       = "Waiting for KES TLS Certificate"
	StatusWaitingConsoleCert                   = "Waiting for Console TLS Certificate"
	StatusUpdatingMinIOVersion                 = "Updating MinIO Version"
	StatusUpdatingConsole                      = "Updating Console"
	StatusUpdatingKES                          = "Updating KES"
	StatusUpdatingLogPGStatefulSet             = "Updating Postgres server for Log Search feature"
	StatusUpdatingLogSearchAPIServer           = "Updating Log Search API server"
	StatusUpdatingResourceRequirements         = "Updating Resource Requirements"
	StatusUpdatingAffinity                     = "Updating Pod Affinity"
	StatusNotOwned                             = "Statefulset not controlled by operator"
	StatusFailedAlreadyExists                  = "Another MinIO Tenant already exists in the namespace"
	StatusInconsistentMinIOVersions            = "Different versions across MinIO Pools"
)

// ErrMinIONotReady is the error returned when MinIO is not Ready
var ErrMinIONotReady = fmt.Errorf("MinIO is not ready")

// ErrLogSearchNotReady is the error returned when Log Search is not Ready
var ErrLogSearchNotReady = fmt.Errorf("Log Search is not ready")

// Controller struct watches the Kubernetes API for changes to Tenant resources
type Controller struct {
	// kubeClientSet is a standard kubernetes clientset
	kubeClientSet kubernetes.Interface
	// minioClientSet is a clientset for our own API group
	minioClientSet clientset.Interface
	// certClient is a clientset for our certficate management
	certClient certapi.CertificatesV1Client
	// promClient is a clientset for Prometheus service monitor
	promClient promclientset.Interface
	// statefulSetLister is able to list/get StatefulSets from a shared
	// informer's store.
	statefulSetLister appslisters.StatefulSetLister
	// statefulSetListerSynced returns true if the StatefulSet shared informer
	// has synced at least once.
	statefulSetListerSynced cache.InformerSynced

	// deploymentLister is able to list/get Deployments from a shared
	// informer's store.
	deploymentLister appslisters.DeploymentLister
	// deploymentListerSynced returns true if the Deployment shared informer
	// has synced at least once.
	deploymentListerSynced cache.InformerSynced

	// jobLister is able to list/get Deployments from a shared
	// informer's store.
	jobLister batchlisters.JobLister
	// jobListerSynced returns true if the Deployment shared informer
	// has synced at least once.
	jobListerSynced cache.InformerSynced

	// tenantsLister lists Tenant from a shared informer's
	// store.
	tenantsLister listers.TenantLister
	// tenantsSynced returns true if the StatefulSet shared informer
	// has synced at least once.
	tenantsSynced cache.InformerSynced

	// serviceLister is able to list/get Services from a shared informer's
	// store.
	serviceLister corelisters.ServiceLister
	// serviceListerSynced returns true if the Service shared informer
	// has synced at least once.
	serviceListerSynced cache.InformerSynced

	// serviceMonitorLister is able to list/get Services from a shared informer's
	// store.
	serviceMonitorLister promlisters.ServiceMonitorLister
	// serviceMonitorListerSynced returns true if the Service shared informer
	// has synced at least once.
	serviceMonitorListerSynced cache.InformerSynced

	// queue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue queue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	// Use a go template to render the hosts string
	hostsTemplate string

	// currently running operator version
	operatorVersion string

	// Webhook server instance
	ws *http.Server
}

// NewController returns a new sample controller
func NewController(
	kubeClientSet kubernetes.Interface,
	minioClientSet clientset.Interface,
	certClient certapi.CertificatesV1Client,
	promClient promclientset.Interface,
	statefulSetInformer appsinformers.StatefulSetInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	jobInformer batchinformers.JobInformer,
	tenantInformer informers.TenantInformer,
	serviceInformer coreinformers.ServiceInformer,
	serviceMonitorInformer prominformers.ServiceMonitorInformer,
	hostsTemplate, operatorVersion string) *Controller {

	// Create event broadcaster
	// Add minio-controller types to the default Kubernetes Scheme so Events can be
	// logged for minio-controller types.
	minioscheme.AddToScheme(scheme.Scheme) //nolint:errcheck
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeClientSet:              kubeClientSet,
		minioClientSet:             minioClientSet,
		certClient:                 certClient,
		promClient:                 promClient,
		statefulSetLister:          statefulSetInformer.Lister(),
		statefulSetListerSynced:    statefulSetInformer.Informer().HasSynced,
		deploymentLister:           deploymentInformer.Lister(),
		deploymentListerSynced:     deploymentInformer.Informer().HasSynced,
		jobLister:                  jobInformer.Lister(),
		jobListerSynced:            jobInformer.Informer().HasSynced,
		tenantsLister:              tenantInformer.Lister(),
		tenantsSynced:              tenantInformer.Informer().HasSynced,
		serviceLister:              serviceInformer.Lister(),
		serviceListerSynced:        serviceInformer.Informer().HasSynced,
		serviceMonitorLister:       serviceMonitorInformer.Lister(),
		serviceMonitorListerSynced: serviceMonitorInformer.Informer().HasSynced,
		workqueue:                  queue.NewNamedRateLimitingQueue(MinIOControllerRateLimiter(), "Tenants"),
		recorder:                   recorder,
		hostsTemplate:              hostsTemplate,
		operatorVersion:            operatorVersion,
	}

	// Initialize operator webhook handlers
	controller.ws = configureWebhookServer(controller)

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Tenant resources change
	tenantInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueTenant,
		UpdateFunc: func(old, new interface{}) {
			oldTenant := old.(*miniov2.Tenant)
			newTenant := new.(*miniov2.Tenant)
			if newTenant.ResourceVersion == oldTenant.ResourceVersion {
				// Periodic resync will send update events for all known Tenants.
				// Two different versions of the same Tenant will always have different RVs.
				return
			}
			controller.enqueueTenant(new)
		},
	})
	// Set up an event handler for when StatefulSet resources change. This
	// handler will lookup the owner of the given StatefulSet, and if it is
	// owned by a Tenant resource will enqueue that Tenant resource for
	// processing. This way, we don't need to implement custom logic for
	// handling StatefulSet resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/controllers.md
	statefulSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.StatefulSet)
			oldDepl := old.(*appsv1.StatefulSet)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known StatefulSet.
				// Two different versions of the same StatefulSet will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployments will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})
	return controller
}

func (c *Controller) validateRequest(r *http.Request, secret *v1.Secret) error {
	tokenStr, err := jwtreq.AuthorizationHeaderExtractor.ExtractToken(r)
	if err != nil {
		return err
	}

	stdClaims := &jwt.StandardClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, stdClaims, func(token *jwt.Token) (interface{}, error) {
		return secret.Data[miniov2.WebhookOperatorPassword], nil
	})
	if err != nil {
		return err
	}

	if !token.Valid {
		return fmt.Errorf(http.StatusText(http.StatusForbidden))
	}
	if stdClaims.Issuer != string(secret.Data[miniov2.WebhookOperatorUsername]) {
		return fmt.Errorf(http.StatusText(http.StatusForbidden))
	}

	return nil
}

func generateRandomKey(length int) string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZÅÄÖ" +
		"abcdefghijklmnopqrstuvwxyzåäö" +
		"0123456789")
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

func (c *Controller) applyOperatorWebhookSecret(ctx context.Context, tenant *miniov2.Tenant) (*v1.Secret, error) {
	secret, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Get(ctx,
		miniov2.WebhookSecret, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			secret = getSecretForTenant(tenant, generateRandomKey(20), generateRandomKey(40))
			return c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		}
		return nil, err
	}
	// check the secret has the desired values
	minioArgs := string(secret.Data[miniov2.WebhookMinIOArgs])
	if strings.Contains(minioArgs, "env://") {
		// update the secret
		minioArgs = strings.ReplaceAll(minioArgs, "env://", "env+tls://")
		secret.Data[miniov2.WebhookMinIOArgs] = []byte(minioArgs)
		secret, err = c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		// update the revision of the tenant to force a rolling restart across all statefulsets
		t2, err := c.increaseTenantRevision(ctx, tenant)
		if err != nil {
			return nil, err
		}
		*tenant = *t2
	}

	return secret, nil
}

func getSecretForTenant(tenant *miniov2.Tenant, accessKey, secretKey string) *v1.Secret {
	secret := &corev1.Secret{
		Type: "Opaque",
		ObjectMeta: metav1.ObjectMeta{
			Name:      miniov2.WebhookSecret,
			Namespace: tenant.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(tenant, schema.GroupVersionKind{
					Group:   miniov2.SchemeGroupVersion.Group,
					Version: miniov2.SchemeGroupVersion.Version,
					Kind:    miniov2.MinIOCRDResourceKind,
				}),
			},
		},
		Data: map[string][]byte{
			miniov2.WebhookOperatorUsername: []byte(accessKey),
			miniov2.WebhookOperatorPassword: []byte(secretKey),
			miniov2.WebhookMinIOArgs: []byte(fmt.Sprintf("%s://%s:%s@%s:%s%s/%s/%s",
				"env+tls",
				accessKey,
				secretKey,
				fmt.Sprintf("operator.%s.svc.%s",
					miniov2.GetNSFromFile(),
					miniov2.GetClusterDomain()),
				miniov2.WebhookDefaultPort,
				miniov2.WebhookAPIGetenv,
				tenant.Namespace,
				tenant.Name)),
		},
	}
	return secret
}

func (c *Controller) fetchTag(path string) (string, error) {
	cmd := exec.Command(path, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	op := strings.Fields(out.String())
	if len(op) != 3 {
		return "", fmt.Errorf("incorrect output while fetching tag value - %d", len((op)))
	}
	return op[2], nil
}

// minioKeychain implements Keychain to pass custom credentials
type minioKeychain struct {
	authn.Keychain
	Username      string
	Password      string
	Auth          string
	IdentityToken string
	RegistryToken string
}

// Resolve implements Keychain.
func (mk *minioKeychain) Resolve(_ authn.Resource) (authn.Authenticator, error) {
	return authn.FromConfig(authn.AuthConfig{
		Username:      mk.Username,
		Password:      mk.Password,
		Auth:          mk.Auth,
		IdentityToken: mk.IdentityToken,
		RegistryToken: mk.RegistryToken,
	}), nil
}

// getKeychainForTenant attempts to build a new authn.Keychain from the image pull secret on the Tenant
func (c *Controller) getKeychainForTenant(ctx context.Context, ref name.Reference, tenant *miniov2.Tenant) (authn.Keychain, error) {
	// Get the secret
	secret, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Get(ctx, tenant.Spec.ImagePullSecret.Name, metav1.GetOptions{})
	if err != nil {
		return authn.DefaultKeychain, errors.New("can't retrieve the tenant image pull secret")
	}
	// if we can't find .dockerconfigjson, error out
	dockerConfigJSON, ok := secret.Data[".dockerconfigjson"]
	if !ok {
		return authn.DefaultKeychain, fmt.Errorf("unable to find `.dockerconfigjson` in image pull secret")
	}
	var config configfile.ConfigFile
	if err = json.Unmarshal(dockerConfigJSON, &config); err != nil {
		return authn.DefaultKeychain, fmt.Errorf("Unable to decode docker config secrets %w", err)
	}
	cfg, ok := config.AuthConfigs[ref.Context().RegistryStr()]
	if !ok {
		return authn.DefaultKeychain, fmt.Errorf("unable to locate auth config registry context %s", ref.Context().RegistryStr())
	}
	return &minioKeychain{
		Username:      cfg.Username,
		Password:      cfg.Password,
		Auth:          cfg.Auth,
		IdentityToken: cfg.IdentityToken,
		RegistryToken: cfg.RegistryToken,
	}, nil
}

// Attempts to fetch given image and then extracts and keeps relevant files
// (minio, minio.sha256sum & minio.minisig) at a pre-defined location (/tmp/webhook/v1/update)
func (c *Controller) fetchArtifacts(tenant *miniov2.Tenant) (latest time.Time, err error) {
	basePath := updatePath

	if err = os.MkdirAll(basePath, 1777); err != nil {
		return latest, err
	}

	ref, err := name.ParseReference(tenant.Spec.Image)
	if err != nil {
		return latest, err
	}

	keychain := authn.DefaultKeychain

	// if the tenant has imagePullSecret use that for pulling the image, but if we fail to extract the secret or we
	// can't find the expected registry in the secret we will continue with the default keychain. This is because the
	// needed pull secret could be attached to the service-account.
	if tenant.Spec.ImagePullSecret.Name != "" {
		// Get the secret
		keychain, err = c.getKeychainForTenant(context.Background(), ref, tenant)
		if err != nil {
			klog.Info(err)
		}
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(keychain))
	if err != nil {
		return latest, err
	}

	ls, err := img.Layers()
	if err != nil {
		return latest, err
	}

	// Find the file with largest size among all layers.
	// This is the tar file with all minio relevant files.
	start := 0
	if len(ls) >= 2 { // skip the base layer
		start = 1
	}
	maxSizeHash, _ := ls[start].Digest()
	maxSize, _ := ls[start].Size()
	for i := range ls {
		if i < start {
			continue
		}
		s, _ := ls[i].Size()
		if s > maxSize {
			maxSize, _ = ls[i].Size()
			maxSizeHash, _ = ls[i].Digest()
		}
	}

	f, err := os.OpenFile(basePath+"image.tar", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return latest, err
	}
	defer func() {
		_ = f.Close()
	}()

	// Tarball writes a file called image.tar
	// This file in turn has each container layer present inside in the form `<layer-hash>.tar.gz`
	if err = tarball.Write(ref, img, f); err != nil {
		return latest, err
	}

	// Extract the <layer-hash>.tar.gz file that has minio contents from `image.tar`
	fileNameToExtract := strings.Split(maxSizeHash.String(), ":")[1] + ".tar.gz"
	if err = miniov2.ExtractTar([]string{fileNameToExtract}, basePath, "image.tar"); err != nil {
		return latest, err
	}

	// Extract the minio update related files (minio, minio.sha256sum and minio.minisig) from `<layer-hash>.tar.gz`
	if err = miniov2.ExtractTar([]string{"usr/bin/minio", "usr/bin/minio.sha256sum", "usr/bin/minio.minisig"}, basePath, fileNameToExtract); err != nil {
		return latest, err
	}

	srcBinary := "minio"
	srcShaSum := "minio.sha256sum"
	srcSig := "minio.minisig"

	tag, err := c.fetchTag(basePath + srcBinary)
	if err != nil {
		return latest, err
	}

	latest, err = miniov2.ReleaseTagToReleaseTime(tag)
	if err != nil {
		return latest, err
	}

	destBinary := "minio." + tag
	destShaSum := "minio." + tag + ".sha256sum"
	destSig := "minio." + tag + ".minisig"
	filesToRename := map[string]string{srcBinary: destBinary, srcShaSum: destShaSum, srcSig: destSig}

	// rename all files to add tag specific values in the name.
	// this is because minio updater looks for files in this name format.
	for s, d := range filesToRename {
		if err = os.Rename(basePath+s, basePath+d); err != nil {
			return latest, err
		}
	}
	return latest, nil
}

// Remove all the files created during upload process
func (c *Controller) removeArtifacts() error {
	return os.RemoveAll(updatePath)
}

var operatorTLSSecretName = "operator-tls"

// Start will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Start(threadiness int, stopCh <-chan struct{}) error {
	go func() {
		ctx := context.Background()
		namespace := miniov2.GetNSFromFile()
		// operator deployment for owner reference
		operatorDeployment, err := c.kubeClientSet.AppsV1().Deployments(namespace).Get(ctx, "minio-operator", metav1.GetOptions{})
		if err != nil {
			panic(err)
		}

		publicCertPath := "/tmp/public.crt"
		publicKeyPath := "/tmp/private.key"

		for {
			// operator TLS certificates
			operatorTLSCert, err := c.kubeClientSet.CoreV1().Secrets(namespace).Get(ctx, operatorTLSSecretName, metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					klog.Infof("operator TLS secret not found", err.Error())
					if err = c.checkAndCreateOperatorCSR(ctx, operatorDeployment); err != nil {
						klog.Infof("Waiting for the operator certificates to be issued %v", err.Error())
						time.Sleep(time.Second * 10)
					} else {
						if err = c.certClient.CertificateSigningRequests().Delete(ctx, "operator-auto-tls", metav1.DeleteOptions{}); err != nil {
							klog.Infof(err.Error())
						}
					}
				}
			} else {
				if val, ok := operatorTLSCert.Data["public.crt"]; ok {
					err := ioutil.WriteFile(publicCertPath, val, 0644)
					if err != nil {
						panic(err)
					}
				} else {
					panic(errors.New("operator TLS wrong format"))
				}

				if val, ok := operatorTLSCert.Data["private.key"]; ok {
					err := ioutil.WriteFile(publicKeyPath, val, 0644)
					if err != nil {
						panic(err)
					}
				} else {
					panic(errors.New("operator TLS wrong format"))
				}
				break
			}
		}
		klog.Infof("Starting api server")
		// use those certificates to configure the web server
		if err := c.ws.ListenAndServeTLS(publicCertPath, publicKeyPath); err != http.ErrServerClosed {
			klog.Infof("HTTPS server ListenAndServeTLS: %v", err)
			return
		}
	}()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Tenant controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.statefulSetListerSynced, c.deploymentListerSynced, c.tenantsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process Tenant resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	// Launch a goroutine to monitor all Tenants
	go c.recurrentTenantStatusMonitor(stopCh)

	return nil
}

// Stop is called to shutdown the controller
func (c *Controller) Stop() {
	klog.Info("Stopping the minio controller webhook")
	// Wait upto 5 secs and terminate all connections.
	tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = c.ws.Shutdown(tctx)
	cancel()

	klog.Info("Stopping the minio controller")
	c.workqueue.ShutDown()
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	defer runtime.HandleCrash()
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	processItem := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		klog.V(2).Infof("Key from workqueue: %s", key)
		// Run the syncHandler, passing it the namespace/name string of the
		// Tenant resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}

	if err := processItem(obj); err != nil {
		runtime.HandleError(err)
		return true
	}
	return true
}

const slashSeparator = "/"

func key2NamespaceName(key string) (namespace, name string) {
	key = strings.TrimPrefix(key, slashSeparator)
	m := strings.Index(key, slashSeparator)
	if m < 0 {
		return "", key
	}
	return key[:m], key[m+len(slashSeparator):]
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Tenant resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	ctx := context.Background()
	cOpts := metav1.CreateOptions{}
	uOpts := metav1.UpdateOptions{}
	gOpts := metav1.GetOptions{}

	// Convert the namespace/name string into a distinct namespace and name
	if key == "" {
		runtime.HandleError(fmt.Errorf("Invalid resource key: %s", key))
		return nil
	}

	namespace, tenantName := key2NamespaceName(key)

	// Get the Tenant resource with this namespace/name
	tenant, err := c.tenantsLister.Tenants(namespace).Get(tenantName)
	if err != nil {
		// The Tenant resource may no longer exist, in which case we stop processing.
		if k8serrors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("Tenant '%s' in work queue no longer exists", key))
			return nil
		}
		return nil
	}
	// Set any required default values and init Global variables
	nsName := types.NamespacedName{Namespace: namespace, Name: tenantName}

	tenant.EnsureDefaults()

	// Validate the MinIO Tenant
	if err = tenant.Validate(); err != nil {
		klog.V(2).Infof(err.Error())
		var err2 error
		if _, err2 = c.updateTenantStatus(ctx, tenant, err.Error(), 0); err2 != nil {
			klog.V(2).Infof(err2.Error())
		}
		// return nil so we don't re-queue this work item
		return nil
	}
	// AutoCertEnabled verification is used to manage the tenant migration between v1 and v2
	// Previous behavior was that AutoCert is disabled by default if RequestAutoCert is nil
	// New behavior is that AutoCert is enabled by default if RequestAutoCert is nil
	// In the future this support will be dropped
	if tenant.Status.Certificates.AutoCertEnabled == nil {
		autoCertEnabled := true
		if tenant.Spec.RequestAutoCert == nil && tenant.APIVersion != "" {
			// If we get certificate signing requests for MinIO is safe to assume the Tenant v1 was deployed using AutoCert
			// otherwise AutoCert will be false
			tenantCSR, err := c.certClient.CertificateSigningRequests().Get(ctx, tenant.MinIOCSRName(), metav1.GetOptions{})
			if err != nil || tenantCSR == nil {
				autoCertEnabled = false
			}
		} else {
			autoCertEnabled = tenant.AutoCert()
		}
		if tenant, err = c.updateCertificatesStatus(ctx, tenant, autoCertEnabled); err != nil {
			klog.V(2).Infof(err.Error())
		}
	}

	secret, err := c.applyOperatorWebhookSecret(ctx, tenant)
	if err != nil {
		return err
	}

	// validate the minio service
	err = c.checkMinIOSCertificatesStatus(ctx, tenant, nsName)
	if err != nil {
		klog.V(2).Infof("Error when consolidating tenant service: %v", err)
		return err
	}

	// Handle the Internal Headless Service for Tenant StatefulSet
	hlSvc, err := c.serviceLister.Services(tenant.Namespace).Get(tenant.MinIOHLServiceName())
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningHLService, 0); err != nil {
				return err
			}
			klog.V(2).Infof("Creating a new Headless Service for cluster %q", nsName)
			// Create the headless service for the tenant
			hlSvc = services.NewHeadlessForMinIO(tenant)
			_, err = c.kubeClientSet.CoreV1().Services(tenant.Namespace).Create(ctx, hlSvc, cOpts)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// List all MinIO Tenants in this namespace.
	li, err := c.tenantsLister.Tenants(tenant.Namespace).List(labels.NewSelector())
	if err != nil {
		return err
	}

	// Only 1 minio tenant per namespace allowed.
	if len(li) > 1 {
		for _, t := range li {
			if t.Status.CurrentState != StatusInitialized {
				if _, err = c.updateTenantStatus(ctx, t, StatusFailedAlreadyExists, 0); err != nil {
					return err
				}
				// return nil so we don't re-queue this work item
				return nil
			}
		}
	}

	var tenantConfiguration map[string][]byte

	if tenant.HasConfigurationSecret() {
		minioConfigurationSecretName := tenant.Spec.Configuration.Name
		minioConfigurationSecret, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Get(ctx, minioConfigurationSecretName, gOpts)
		if err != nil {
			return err
		}
		tenantConfiguration = miniov2.ParseRawConfiguration(minioConfigurationSecret.Data["config.env"])
	} else {
		minioSecretName := tenant.Spec.CredsSecret.Name
		minioSecret, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Get(ctx, minioSecretName, gOpts)
		if err != nil {
			return err
		}
		tenantConfiguration = minioSecret.Data
	}
	adminClnt, err := tenant.NewMinIOAdmin(tenantConfiguration)
	if err != nil {
		return err
	}
	// For each pool check if there is a stateful set
	var totalReplicas int32
	var images []string

	err = c.checkKESStatus(ctx, tenant, totalReplicas, cOpts, uOpts, nsName)
	if err != nil {
		klog.V(2).Infof("Error checking KES state %v", err)
		return err
	}

	// Copy Operator TLS certificate to Tenant Namespace
	operatorTLSSecret, err := c.kubeClientSet.CoreV1().Secrets(miniov2.GetNSFromFile()).Get(ctx, operatorTLSSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if val, ok := operatorTLSSecret.Data["public.crt"]; ok {
		secret := &corev1.Secret{
			Type: "Opaque",
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorTLSSecretName,
				Namespace: tenant.Namespace,
				Labels:    tenant.MinIOPodLabels(),
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(tenant, schema.GroupVersionKind{
						Group:   miniov2.SchemeGroupVersion.Group,
						Version: miniov2.SchemeGroupVersion.Version,
						Kind:    miniov2.MinIOCRDResourceKind,
					}),
				},
			},
			Data: map[string][]byte{
				"public.crt": val,
			},
		}
		_, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return err
		}
	}
	// consolidate the status of all pools. this is meant to cover for legacy tenants
	// this status value is zero only for new tenants or legacy tenants
	if len(tenant.Status.Pools) == 0 {
		poolDir, err := c.getAllSSForTenant(tenant)
		if err != nil {
			return err
		}
		for pi := range poolDir {
			if poolDir[pi] != nil {
				tenant.Status.Pools = append(tenant.Status.Pools, miniov2.PoolStatus{
					SSName: poolDir[pi].Name,
					State:  miniov2.PoolCreated,
				})
			}
		}
		// push updates to status
		if tenant, err = c.updatePoolStatus(ctx, tenant); err != nil {
			return err
		}
	}

	// Check if this is fresh setup not an expansion.
	freshSetup := len(tenant.Spec.Pools) == len(tenant.Status.Pools)
	for i, pool := range tenant.Spec.Pools {
		// Get the StatefulSet with the name specified in Tenant.status.pools[i].SSName

		// if this index is in the status of pools use it, else capture the desired name in the status and store it
		var ssName string
		if len(tenant.Status.Pools) > i {
			ssName = tenant.Status.Pools[i].SSName
		} else {
			ssName = tenant.PoolStatefulsetName(&pool)
			tenant.Status.Pools = append(tenant.Status.Pools, miniov2.PoolStatus{
				SSName: ssName,
				State:  miniov2.PoolNotCreated,
			})
			// push updates to status
			if tenant, err = c.updatePoolStatus(ctx, tenant); err != nil {
				return err
			}
		}
		ss, err := c.statefulSetLister.StatefulSets(tenant.Namespace).Get(ssName)
		if k8serrors.IsNotFound(err) {

			klog.Infof("Deploying pool %s", pool.Name)

			// Check healthcheck for previous pool only if its not a fresh setup,
			// if they are online before adding this pool.
			if !freshSetup && !tenant.MinIOHealthCheck() {
				klog.Infof("Deploying pool failed %s", pool.Name)
				return ErrMinIONotReady
			}

			if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningStatefulSet, 0); err != nil {
				return err
			}

			ss = statefulsets.NewPool(tenant, secret, &pool, hlSvc.Name, c.hostsTemplate, c.operatorVersion)
			ss, err = c.kubeClientSet.AppsV1().StatefulSets(tenant.Namespace).Create(ctx, ss, cOpts)
			if err != nil {
				return err
			}

			// Report the pool is properly created
			tenant.Status.Pools[i].State = miniov2.PoolCreated
			// push updates to status
			if tenant, err = c.updatePoolStatus(ctx, tenant); err != nil {
				return err
			}
			// Restart the services to fetch the new args, ignore any error.
			// only perform `restart()` of server deployment when we are truly
			// expanding an existing deployment.
			if !freshSetup {
				adminClnt.ServiceRestart(ctx) //nolint:errcheck
			}
		} else {
			if pool.Servers != *ss.Spec.Replicas {
				// warn the user that replica count of an existing pool can't be changed
				if tenant, err = c.updateTenantStatus(ctx, tenant, fmt.Sprintf("Can't modify server count for pool %s", pool.Name), 0); err != nil {
					return err
				}
			}
			// Verify if this pool matches the spec on the tenant (resources, affinity, sidecars, etc)
			poolMatchesSS, err := poolSSMatchesSpec(tenant, &pool, ss, c.operatorVersion)
			if err != nil {
				return err
			}
			// if the pool doesn't match the spec
			if !poolMatchesSS {
				// for legacy reasons, if the zone label is present in SS we must carry it over
				carryOverLabels := make(map[string]string)
				if val, ok := ss.Spec.Template.ObjectMeta.Labels[miniov1.ZoneLabel]; ok {
					carryOverLabels[miniov1.ZoneLabel] = val
				}

				nss := statefulsets.NewPool(tenant, secret, &pool, hlSvc.Name, c.hostsTemplate, c.operatorVersion)
				ssCopy := ss.DeepCopy()

				ssCopy.Spec.Template = nss.Spec.Template
				ssCopy.Spec.UpdateStrategy = nss.Spec.UpdateStrategy

				if ss.Spec.Template.ObjectMeta.Labels == nil {
					ssCopy.Spec.Template.ObjectMeta.Labels = make(map[string]string)
				}
				for k, v := range carryOverLabels {
					ssCopy.Spec.Template.ObjectMeta.Labels[k] = v
				}

				if ss, err = c.kubeClientSet.AppsV1().StatefulSets(tenant.Namespace).Update(ctx, ssCopy, uOpts); err != nil {
					return err
				}
			}

		}
		// If the StatefulSet is not controlled by this Tenant resource, we should log
		// a warning to the event recorder and ret
		if !metav1.IsControlledBy(ss, tenant) {
			if tenant, err = c.updateTenantStatus(ctx, tenant, StatusNotOwned, ss.Status.Replicas); err != nil {
				return err
			}
			msg := fmt.Sprintf(MessageResourceExists, ss.Name)
			c.recorder.Event(tenant, corev1.EventTypeWarning, ErrResourceExists, msg)
			// return nil so we don't re-queue this work item, this error won't get fixed by reprocessing
			return nil
		}

		// keep track of all replicas
		totalReplicas += ss.Status.Replicas
		images = append(images, ss.Spec.Template.Spec.Containers[0].Image)
	}
	// validate each pool if it's initialized
	for pi, pool := range tenant.Spec.Pools {
		// get a pod for the established statefulset
		if tenant.Status.Pools[pi].State == miniov2.PoolCreated {
			// get a pod for the ss
			pods, err := c.kubeClientSet.CoreV1().Pods(tenant.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", miniov2.PoolLabel, pool.Name),
			})
			if err != nil {
				klog.Warning("Could not validate state of statefulset for pool", err)
			}
			if len(pods.Items) > 0 {
				ssPod := pods.Items[0]
				podAddress := fmt.Sprintf("%s:9000", tenant.MinIOHLPodHostname(ssPod.Name))
				podAdminClnt, err := tenant.NewMinIOAdminForAddress(podAddress, tenantConfiguration)
				if err != nil {
					return err
				}

				_, err = podAdminClnt.ServerInfo(ctx)
				// any error means we are not ready, if the call succeeds, the ss is ready
				if err == nil {
					// Report the pool is properly created
					tenant.Status.Pools[pi].State = miniov2.PoolInitialized
					// push updates to status
					if tenant, err = c.updatePoolStatus(ctx, tenant); err != nil {
						return err
					}
				} else {
					fmt.Println(err)
				}
			}
		}
	}

	// compare all the images across all pools, they should always be the same.
	for _, image := range images {
		for i := 0; i < len(images); i++ {
			if image != images[i] {
				if _, err = c.updateTenantStatus(ctx, tenant, StatusInconsistentMinIOVersions, totalReplicas); err != nil {
					return err
				}
				return fmt.Errorf("Pool %d is running incorrect image version, all pools are required to be on the same MinIO version. Attempting update of the inconsistent pool",
					i+1)
			}
		}
	}

	// In loop above we compared all the versions in all pools.
	// So comparing tenant.Spec.Image (version to update to) against one value from images slice is fine.
	if tenant.Spec.Image != images[0] && tenant.Status.CurrentState != StatusUpdatingMinIOVersion {
		if !tenant.MinIOHealthCheck() {
			return ErrMinIONotReady
		}

		// Images different with the newer state change, continue to verify
		// if upgrade is possible
		tenant, err = c.updateTenantStatus(ctx, tenant, StatusUpdatingMinIOVersion, totalReplicas)
		if err != nil {
			return err
		}

		klog.V(4).Infof("Collecting artifacts for Tenant '%s' to update MinIO from: %s, to: %s",
			tenantName, images[0], tenant.Spec.Image)

		latest, err := c.fetchArtifacts(tenant)
		if err != nil {
			_ = c.removeArtifacts()
			return err
		}
		updateURL, err := tenant.UpdateURL(latest, fmt.Sprintf("http://operator.%s.svc.%s:%s%s",
			miniov2.GetNSFromFile(), miniov2.GetClusterDomain(),
			miniov2.WebhookDefaultPort, miniov2.WebhookAPIUpdate,
		))
		if err != nil {
			_ = c.removeArtifacts()

			err = fmt.Errorf("Unable to get canonical update URL for Tenant '%s', failed with %v", tenantName, err)
			if _, terr := c.updateTenantStatus(ctx, tenant, err.Error(), totalReplicas); terr != nil {
				return terr
			}

			// Correct URL could not be obtained, not proceeding to update.
			return err
		}

		klog.V(4).Infof("Updating Tenant %s MinIO version from: %s, to: %s -> URL: %s",
			tenantName, tenant.Spec.Image, images[0], updateURL)

		us, err := adminClnt.ServerUpdate(ctx, updateURL)
		if err != nil {
			_ = c.removeArtifacts()

			err = fmt.Errorf("Tenant '%s' MinIO update failed with %w", tenantName, err)
			if _, terr := c.updateTenantStatus(ctx, tenant, err.Error(), totalReplicas); terr != nil {
				return terr
			}

			// Update failed, nothing needs to be changed in the container
			return err
		}

		if us.CurrentVersion != us.UpdatedVersion {
			klog.Infof("Tenant '%s' MinIO updated successfully from: %s, to: %s successfully",
				tenantName, us.CurrentVersion, us.UpdatedVersion)
		} else {
			msg := fmt.Sprintf("Tenant '%s' MinIO is already running the most recent version of %s",
				tenantName,
				us.CurrentVersion)
			klog.Info(msg)
			if _, terr := c.updateTenantStatus(ctx, tenant, msg, totalReplicas); terr != nil {
				return err
			}
			return nil
		}

		// clean the local directory
		_ = c.removeArtifacts()

		for _, pool := range tenant.Spec.Pools {
			// Now proceed to make the yaml changes for the tenant statefulset.
			ss := statefulsets.NewPool(tenant, secret, &pool, hlSvc.Name, c.hostsTemplate, c.operatorVersion)
			if _, err = c.kubeClientSet.AppsV1().StatefulSets(tenant.Namespace).Update(ctx, ss, uOpts); err != nil {
				return err
			}
		}

	}

	// Check whether console is enabled or if it should be removed and the state of it's service
	err = c.checkConsoleStatus(ctx, tenant, totalReplicas, adminClnt, cOpts, uOpts, nsName)
	if err != nil {
		klog.V(2).Infof("Error checking console state %v", err)
		return err
	}

	if tenant.HasLogEnabled() {
		var logSecret *corev1.Secret
		logSecret, err = c.checkAndCreateLogSecret(ctx, tenant)
		if err != nil {
			return err
		}

		searchSvc, err := c.checkAndCreateLogHeadless(ctx, tenant)
		if err != nil {
			return err
		}

		err = c.checkAndCreateLogStatefulSet(ctx, tenant, searchSvc.Name)
		if err != nil {
			return err
		}

		err = c.checkAndCreateLogSearchAPIDeployment(ctx, tenant)
		if err != nil {
			return err
		}

		err = c.checkAndCreateLogSearchAPIService(ctx, tenant)
		if err != nil {
			return err
		}
		// Make sure that MinIO is up and running to enable Log Search.
		if !tenant.MinIOHealthCheck() {
			if _, err = c.updateTenantStatus(ctx, tenant, StatusWaitingForReadyState, totalReplicas); err != nil {
				return err
			}
			return ErrMinIONotReady
		}
		err = c.checkAndConfigureLogSearchAPI(ctx, tenant, logSecret, adminClnt)
		if err != nil {
			return err
		}
	}

	if tenant.HasPrometheusEnabled() {
		_, err := c.checkAndCreatePrometheusConfigMap(ctx, tenant, string(tenantConfiguration["accesskey"]), string(tenantConfiguration["secretkey"]))
		if err != nil {
			return err
		}

		_, err = c.checkAndCreatePrometheusHeadless(ctx, tenant)
		if err != nil {
			return err
		}

		err = c.checkAndCreatePrometheusStatefulSet(ctx, tenant)
		if err != nil {
			return err
		}
	}

	if tenant.HasPrometheusSMEnabled() {
		err = c.checkAndCreatePrometheusServiceMonitorSecret(ctx, tenant, string(tenantConfiguration["accesskey"]), string(tenantConfiguration["secretkey"]))
		if err != nil {
			return err
		}
		err = c.checkAndCreatePrometheusServiceMonitor(ctx, tenant)
		if err != nil {
			return err
		}
	}

	// Finally, we update the status block of the Tenant resource to reflect the
	// current state of the world
	_, err = c.updateTenantStatus(ctx, tenant, StatusInitialized, totalReplicas)
	return err
}

// enqueueTenant takes a Tenant resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Tenant.
func (c *Controller) enqueueTenant(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Tenant resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Tenant resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Tenant, we should not do anything more
		// with it.
		if ownerRef.Kind != "Tenant" {
			return
		}

		tenant, err := c.tenantsLister.Tenants(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.V(4).Infof("ignoring orphaned object '%s' of tenant '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueTenant(tenant)
		return
	}
}

// MinIOControllerRateLimiter is a no-arg constructor for a default rate limiter for a workqueue for our controller.
// both overall and per-item rate limiting.  The overall is a token bucket and the per-item is exponential
func MinIOControllerRateLimiter() queue.RateLimiter {
	return queue.NewMaxOfRateLimiter(
		queue.NewItemExponentialFailureRateLimiter(5*time.Second, 60*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&queue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}

func (c *Controller) checkAndCreateLogHeadless(ctx context.Context, tenant *miniov2.Tenant) (*corev1.Service, error) {
	svc, err := c.serviceLister.Services(tenant.Namespace).Get(tenant.LogHLServiceName())
	if err == nil || !k8serrors.IsNotFound(err) {
		return svc, err
	}

	klog.V(2).Infof("Creating a new Log Headless Service for %s", tenant.Namespace)
	svc = services.NewHeadlessForLog(tenant)
	_, err = c.kubeClientSet.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	return svc, err
}

func (c *Controller) checkAndCreateLogStatefulSet(ctx context.Context, tenant *miniov2.Tenant, svcName string) error {
	logPgSS, err := c.statefulSetLister.StatefulSets(tenant.Namespace).Get(tenant.LogStatefulsetName())
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningLogPGStatefulSet, 0); err != nil {
			return err
		}

		klog.V(2).Infof("Creating a new Log StatefulSet for %s", tenant.Namespace)
		searchSS := statefulsets.NewForLogDb(tenant, svcName)
		_, err = c.kubeClientSet.AppsV1().StatefulSets(tenant.Namespace).Create(ctx, searchSS, metav1.CreateOptions{})
		return err

	}

	// check if expected and actual values of Log DB spec match
	dbSpecMatches, err := logDBStatefulsetMatchesSpec(tenant, logPgSS)
	if err != nil {
		return err
	}
	if !dbSpecMatches {
		// Note: using current spec replica count works as long as we don't expose replicas via tenant spec.
		if tenant, err = c.updateTenantStatus(ctx, tenant, StatusUpdatingLogPGStatefulSet, *logPgSS.Spec.Replicas); err != nil {
			return err
		}
		logPgSS = statefulsets.NewForLogDb(tenant, svcName)
		if _, err = c.kubeClientSet.AppsV1().StatefulSets(tenant.Namespace).Update(ctx, logPgSS, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) checkAndCreateLogSearchAPIService(ctx context.Context, tenant *miniov2.Tenant) error {
	_, err := c.serviceLister.Services(tenant.Namespace).Get(tenant.LogSearchAPIServiceName())
	if err == nil || !k8serrors.IsNotFound(err) {
		return err
	}

	klog.V(2).Infof("Creating a new Log Search API Service for %s", tenant.Namespace)
	svc := services.NewClusterIPForLogSearchAPI(tenant)
	_, err = c.kubeClientSet.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	return err
}

func (c *Controller) checkAndCreateLogSearchAPIDeployment(ctx context.Context, tenant *miniov2.Tenant) error {
	logSearchDeployment, err := c.deploymentLister.Deployments(tenant.Namespace).Get(tenant.LogSearchAPIDeploymentName())
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningLogSearchAPIDeployment, 0); err != nil {
			return err
		}

		klog.V(2).Infof("Creating a new Log Search API deployment for %s", tenant.Name)
		_, err = c.kubeClientSet.AppsV1().Deployments(tenant.Namespace).Create(ctx, deployments.NewForLogSearchAPI(tenant), metav1.CreateOptions{})
		return err
	}

	// check if expected and actual values of Log search API deployment match
	apiDeploymentMatches, err := logSearchAPIDeploymentMatchesSpec(tenant, logSearchDeployment)
	if err != nil {
		return err
	}
	if !apiDeploymentMatches {
		// Note: using current spec replica count works as long as we don't expose replicas via tenant spec.
		if tenant, err = c.updateTenantStatus(ctx, tenant, StatusUpdatingLogSearchAPIServer, *logSearchDeployment.Spec.Replicas); err != nil {
			return err
		}
		logSearchDeployment = deployments.NewForLogSearchAPI(tenant)
		if _, err := c.kubeClientSet.AppsV1().Deployments(tenant.Namespace).Update(ctx, logSearchDeployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) checkAndCreateLogSecret(ctx context.Context, tenant *miniov2.Tenant) (*corev1.Secret, error) {
	secret, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Get(ctx, tenant.LogSecretName(), metav1.GetOptions{})
	if err == nil || !k8serrors.IsNotFound(err) {
		return secret, err
	}

	klog.V(2).Infof("Creating a new Log secret for %s", tenant.Name)
	secret, err = c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Create(ctx, secrets.LogSecret(tenant), metav1.CreateOptions{})
	return secret, err
}

func (c *Controller) checkAndConfigureLogSearchAPI(ctx context.Context, tenant *miniov2.Tenant, secret *corev1.Secret, adminClnt *madmin.AdminClient) error {
	// Check if audit webhook is configured for tenant's MinIO
	auditCfg := newAuditWebhookConfig(tenant, secret)
	_, err := adminClnt.GetConfigKV(ctx, auditCfg.target)
	if err != nil {
		// check if log search is ready
		if err = c.checkLogSearchAPIReady(tenant); err != nil {
			klog.V(2).Info(err)
			return ErrLogSearchNotReady
		}
		restart, err := adminClnt.SetConfigKV(ctx, auditCfg.args)
		if err != nil {
			return err
		}
		if restart {
			// Restart MinIO for config update to take effect
			if err = adminClnt.ServiceRestart(ctx); err != nil {
				fmt.Println("error restarting minio")
				klog.V(2).Info(err)
			}
			fmt.Println("done restarting minio")
		}
		return nil
	}
	return err
}

func (c *Controller) checkLogSearchAPIReady(tenant *miniov2.Tenant) error {
	endpoint := fmt.Sprintf("http://%s.%s.svc.%s:8080", tenant.LogSearchAPIServiceName(), tenant.Namespace, miniov2.GetClusterDomain())
	client := http.Client{Timeout: 100 * time.Millisecond}
	resp, err := client.Get(endpoint)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.V(2).Info(err)
		}
	}()

	if resp.StatusCode == 404 {
		return nil
	}

	return errors.New("Log Search API Not Ready")
}

func (c *Controller) checkAndCreatePrometheusConfigMap(ctx context.Context, tenant *miniov2.Tenant, accessKey, secretKey string) (*corev1.ConfigMap, error) {
	configMap, err := c.kubeClientSet.CoreV1().ConfigMaps(tenant.Namespace).Get(ctx, tenant.PrometheusConfigMapName(), metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return configMap, err
	} else if err == nil {
		// check if configmap needs update.
		updatedConfigMap := configmaps.UpdatePrometheusConfigMap(tenant, accessKey, secretKey, configMap)
		if updatedConfigMap == nil {
			return configMap, nil
		}

		klog.V(2).Infof("Updating Prometheus config-map for %s", tenant.Name)
		configMap, err = c.kubeClientSet.CoreV1().ConfigMaps(tenant.Namespace).Update(ctx, updatedConfigMap, metav1.UpdateOptions{})
		if err != nil {
			return configMap, err
		}

		return configMap, err
	}

	// otherwise create the config
	klog.V(2).Infof("Creating a new Prometheus config-map for %s", tenant.Name)
	return c.kubeClientSet.CoreV1().ConfigMaps(tenant.Namespace).Create(ctx, configmaps.PrometheusConfigMap(tenant, accessKey, secretKey), metav1.CreateOptions{})
}

func (c *Controller) checkAndCreatePrometheusHeadless(ctx context.Context, tenant *miniov2.Tenant) (*corev1.Service, error) {
	svc, err := c.serviceLister.Services(tenant.Namespace).Get(tenant.PrometheusHLServiceName())
	if err == nil || !k8serrors.IsNotFound(err) {
		return svc, err
	}

	klog.V(2).Infof("Creating a new Prometheus Headless Service for %s", tenant.Namespace)
	svc = services.NewHeadlessForPrometheus(tenant)
	_, err = c.kubeClientSet.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	return svc, err
}

func (c *Controller) checkAndCreatePrometheusStatefulSet(ctx context.Context, tenant *miniov2.Tenant) error {
	_, err := c.statefulSetLister.StatefulSets(tenant.Namespace).Get(tenant.PrometheusStatefulsetName())
	if err == nil || !k8serrors.IsNotFound(err) {
		return err
	}

	if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningPrometheusStatefulSet, 0); err != nil {
		return err
	}

	klog.V(2).Infof("Creating a new Prometheus StatefulSet for %s", tenant.Namespace)
	prometheusSS := statefulsets.NewForPrometheus(tenant, tenant.PrometheusHLServiceName())
	_, err = c.kubeClientSet.AppsV1().StatefulSets(tenant.Namespace).Create(ctx, prometheusSS, metav1.CreateOptions{})
	return err
}

func (c *Controller) checkAndCreatePrometheusServiceMonitorSecret(ctx context.Context, tenant *miniov2.Tenant, accessKey, secretKey string) error {
	_, err := c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Get(ctx, tenant.PromServiceMonitorSecret(), metav1.GetOptions{})
	if err == nil || !k8serrors.IsNotFound(err) {
		return err
	}

	if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningPrometheusServiceMonitor, 0); err != nil {
		return err
	}

	klog.V(2).Infof("Creating a new Prometheus Service Monitor secret for %s", tenant.Namespace)
	secret := secrets.PromServiceMonitorSecret(tenant, accessKey, secretKey)
	_, err = c.kubeClientSet.CoreV1().Secrets(tenant.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	return err
}

func (c *Controller) checkAndCreatePrometheusServiceMonitor(ctx context.Context, tenant *miniov2.Tenant) error {
	_, err := c.serviceMonitorLister.ServiceMonitors(tenant.Namespace).Get(tenant.PrometheusServiceMonitorName())
	if err == nil || !k8serrors.IsNotFound(err) {
		return err
	}

	if tenant, err = c.updateTenantStatus(ctx, tenant, StatusProvisioningPrometheusServiceMonitor, 0); err != nil {
		return err
	}

	klog.V(2).Infof("Creating a new Prometheus Service Monitor for %s", tenant.Namespace)
	prometheusSM := servicemonitor.NewForPrometheus(tenant)
	_, err = c.promClient.MonitoringV1().ServiceMonitors(tenant.Namespace).Create(ctx, prometheusSM, metav1.CreateOptions{})
	return err
}
