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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"

	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"github.com/minio/madmin-go"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Webhook API constants
const (
	WebhookAPIVersion = "/webhook/v1"
)

type hostsTemplateValues struct {
	StatefulSet string
	CIService   string
	HLService   string
	Ellipsis    string
	Domain      string
}

// GetNSFromFile assumes the operator is running inside a k8s pod and extract the
// current namespace from the /var/run/secrets/kubernetes.io/serviceaccount/namespace file
func GetNSFromFile() string {
	namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "minio-operator"
	}
	return string(namespace)
}

// ellipsis returns the host range string
func genEllipsis(start, end int) string {
	return "{" + strconv.Itoa(start) + "..." + strconv.Itoa(end) + "}"
}

// HasCredsSecret returns true if the user has provided a secret
// for a Tenant else false
func (t *Tenant) HasCredsSecret() bool {
	return t.Spec.CredsSecret != nil
}

// HasConfigurationSecret returns true if the user has provided a configuration
// for a Tenant else false
func (t *Tenant) HasConfigurationSecret() bool {
	return t.Spec.Configuration != nil
}

// HasCertConfig returns true if the user has provided a certificate
// config
func (t *Tenant) HasCertConfig() bool {
	return t.Spec.CertConfig != nil
}

// ExternalCert returns true is the user has provided a secret
// that contains CA cert, server cert and server key
func (t *Tenant) ExternalCert() bool {
	return t.Spec.ExternalCertSecret != nil
}

// ExternalClientCert returns true is the user has provided a secret
// that contains CA client cert, server cert and server key
func (t *Tenant) ExternalClientCert() bool {
	return t.Spec.ExternalClientCertSecret != nil
}

// KESExternalCert returns true is the user has provided a secret
// that contains CA cert, server cert and server key for KES pods
func (t *Tenant) KESExternalCert() bool {
	return t.Spec.KES != nil && t.Spec.KES.ExternalCertSecret != nil
}

// KESClientCert returns true is the user has provided a secret
// that contains CA cert, client cert and client key for KES pods
func (t *Tenant) KESClientCert() bool {
	return t.Spec.KES != nil && t.Spec.KES.ClientCertSecret != nil
}

// ConsoleExternalCert returns true is the user has provided a secret
// that contains CA cert, server cert and server key for Console pods
func (t *Tenant) ConsoleExternalCert() bool {
	return t.Spec.Console != nil && t.Spec.Console.ExternalCertSecret != nil
}

// AutoCert is enabled by default, otherwise we return the user provided value
func (t *Tenant) AutoCert() bool {
	if t.Spec.RequestAutoCert == nil {
		return true
	}
	return *t.Spec.RequestAutoCert
}

// VolumePathForZone returns the paths for MinIO mounts based on
// total number of volumes on a given zone
func (t *Tenant) VolumePathForZone(zone *Zone) string {
	if zone.VolumesPerServer == 1 {
		// Add an extra "/" to make sure relative paths are avoided.
		return path.Join("/", t.Spec.Mountpath, "/", t.Spec.Subpath)
	}
	// Add an extra "/" to make sure relative paths are avoided.
	return path.Join("/", t.Spec.Mountpath+genEllipsis(0, int(zone.VolumesPerServer-1)), "/", t.Spec.Subpath)
}

// KESReplicas returns the number of total KES replicas
// required for this cluster
func (t *Tenant) KESReplicas() int32 {
	var replicas int32
	if t.Spec.KES != nil && t.Spec.KES.Replicas != 0 {
		replicas = t.Spec.KES.Replicas
	}
	return replicas
}

const (
	minioReleaseTagTimeLayout = "2006-01-02T15-04-05Z"
	releasePrefix             = "RELEASE"
)

// ReleaseTagToReleaseTime - converts a 'RELEASE.2017-09-29T19-16-56Z.hotfix' into the build time
func ReleaseTagToReleaseTime(releaseTag string) (releaseTime time.Time, err error) {
	fields := strings.Split(releaseTag, ".")
	if len(fields) < 2 || len(fields) > 3 {
		return releaseTime, fmt.Errorf("%s is not a valid release tag", releaseTag)
	}
	if fields[0] != releasePrefix {
		return releaseTime, fmt.Errorf("%s is not a valid release tag", releaseTag)
	}
	return time.Parse(minioReleaseTagTimeLayout, fields[1])
}

// ExtractTar extracts all tar files from the list `filesToExtract` and puts the files in the `basePath` location
func ExtractTar(filesToExtract []string, basePath, tarFileName string) error {
	tarFile, err := os.Open(basePath + tarFileName)
	if err != nil {
		return err
	}
	defer func() {
		_ = tarFile.Close()
	}()

	tr := tar.NewReader(tarFile)
	if strings.HasSuffix(tarFileName, ".gz") {
		gz, err := gzip.NewReader(tarFile)
		if err != nil {
			return err
		}
		defer func() {
			_ = gz.Close()
		}()
		tr = tar.NewReader(gz)
	}

	var success = len(filesToExtract)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			// only success if we have found all files
			if success == 0 {
				break
			}
		}

		if err != nil {
			return fmt.Errorf("Tar file extraction failed for file index: %d, with: %w", success, err)
		}
		if header.Typeflag == tar.TypeReg {
			if name := find(filesToExtract, header.Name); name != "" {
				outFile, err := os.OpenFile(basePath+path.Base(name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
				if err != nil {
					return fmt.Errorf("Tar file extraction failed while opening file: %s, at index: %d, with: %w", name, success, err)
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					_ = outFile.Close()
					return fmt.Errorf("Tar file extraction failed while copying file: %s, at index: %d, with: %w", name, success, err)
				}
				_ = outFile.Close()
				success--
			}
		}
	}
	return nil
}

func find(slice []string, val string) string {
	for _, item := range slice {
		if item == val {
			return item
		}
	}
	return ""
}

// EnsureDefaults will ensure that if a user omits and fields in the
// spec that are required, we set some sensible defaults.
// For example a user can choose to omit the version
// and number of members.
func (t *Tenant) EnsureDefaults() *Tenant {
	if t.Spec.PodManagementPolicy == "" || (t.Spec.PodManagementPolicy != appsv1.OrderedReadyPodManagement &&
		t.Spec.PodManagementPolicy != appsv1.ParallelPodManagement) {
		t.Spec.PodManagementPolicy = miniov2.DefaultPodManagementPolicy
	}

	if t.Spec.Image == "" {
		t.Spec.Image = miniov2.DefaultMinIOImage
	}

	if t.Spec.ImagePullPolicy == "" {
		t.Spec.ImagePullPolicy = miniov2.DefaultImagePullPolicy
	}

	for zi, z := range t.Spec.Zones {
		if z.Name == "" {
			z.Name = fmt.Sprintf("zone-%d", zi)
		}
		t.Spec.Zones[zi] = z
	}

	if t.Spec.Mountpath == "" {
		t.Spec.Mountpath = miniov2.MinIOVolumeMountPath
	}

	if t.Spec.Subpath == "" {
		t.Spec.Subpath = miniov2.MinIOVolumeSubPath
	}

	if t.AutoCert() {
		if t.Spec.CertConfig != nil {
			if t.Spec.CertConfig.CommonName == "" {
				t.Spec.CertConfig.CommonName = t.MinIOWildCardName()
			}
			if t.Spec.CertConfig.DNSNames == nil || len(t.Spec.CertConfig.DNSNames) == 0 {
				t.Spec.CertConfig.DNSNames = t.MinIOHosts()
			}
			if t.Spec.CertConfig.OrganizationName == nil || len(t.Spec.CertConfig.OrganizationName) == 0 {
				t.Spec.CertConfig.OrganizationName = miniov2.DefaultOrgName
			}
		} else {
			t.Spec.CertConfig = &miniov2.CertificateConfig{
				CommonName:       t.MinIOWildCardName(),
				DNSNames:         t.MinIOHosts(),
				OrganizationName: miniov2.DefaultOrgName,
			}
		}
	} else {
		t.Spec.CertConfig = nil
	}

	if t.HasConsoleEnabled() {
		if t.Spec.Console.Image == "" {
			t.Spec.Console.Image = miniov2.DefaultConsoleImage
		}
		if t.Spec.Console.Replicas == 0 {
			t.Spec.Console.Replicas = miniov2.DefaultConsoleReplicas
		}
		if t.Spec.Console.ImagePullPolicy == "" {
			t.Spec.Console.ImagePullPolicy = miniov2.DefaultImagePullPolicy
		}
	}

	if t.HasKESEnabled() {
		if t.Spec.KES.Image == "" {
			t.Spec.KES.Image = miniov2.DefaultKESImage
		}
		if t.Spec.KES.Replicas == 0 {
			t.Spec.KES.Replicas = miniov2.DefaultKESReplicas
		}
		if t.Spec.KES.ImagePullPolicy == "" {
			t.Spec.KES.ImagePullPolicy = miniov2.DefaultImagePullPolicy
		}
		if t.Spec.KES.KeyName == "" {
			t.Spec.KES.KeyName = miniov2.KESMinIOKey
		}
	}

	return t
}

// MinIOEndpoints similar to MinIOHosts but as URLs
func (t *Tenant) MinIOEndpoints(hostsTemplate string) (endpoints []string) {
	hosts := t.MinIOHosts()
	if hostsTemplate != "" {
		hosts = t.TemplatedMinIOHosts(hostsTemplate)
	}

	for _, host := range hosts {
		if t.TLS() {
			endpoints = append(endpoints, "https://"+host)
		} else {
			endpoints = append(endpoints, "http://"+host)
		}
	}

	return endpoints
}

// MinIOHosts returns the domain names in ellipses format created for current Tenant
func (t *Tenant) MinIOHosts() (hosts []string) {
	// Create the ellipses style URL
	for _, z := range t.Spec.Zones {
		if z.Servers == 1 {
			hosts = append(hosts, fmt.Sprintf("%s-%s.%s.%s.svc.%s", t.MinIOStatefulSetNameForZone(&z), "0", t.MinIOHLServiceName(), t.Namespace, ClusterDomain))
		} else {
			hosts = append(hosts, fmt.Sprintf("%s-%s.%s.%s.svc.%s", t.MinIOStatefulSetNameForZone(&z), genEllipsis(0, int(z.Servers)-1), t.MinIOHLServiceName(), t.Namespace, ClusterDomain))
		}
	}
	return hosts
}

// TemplatedMinIOHosts returns the domain names in ellipses format created for current Tenant without the service part
func (t *Tenant) TemplatedMinIOHosts(hostsTemplate string) (hosts []string) {
	tmpl, err := template.New("hosts").Parse(hostsTemplate)
	if err != nil {
		msg := "Invalid go template for hosts"
		klog.V(2).Infof(msg)
		return hosts
	}
	var max, index int32
	// Create the ellipses style URL
	for _, z := range t.Spec.Zones {
		max = max + z.Servers
		data := hostsTemplateValues{
			StatefulSet: t.MinIOStatefulSetNameForZone(&z),
			CIService:   t.MinIOCIServiceName(),
			HLService:   t.MinIOHLServiceName(),
			Ellipsis:    genEllipsis(int(index), int(max)-1),
			Domain:      ClusterDomain,
		}
		output := new(bytes.Buffer)
		if err = tmpl.Execute(output, data); err != nil {
			continue
		}
		hosts = append(hosts, output.String())
		index = max
	}
	return hosts
}

// AllMinIOHosts returns the all the individual domain names relevant for current Tenant
func (t *Tenant) AllMinIOHosts() []string {
	hosts := make([]string, 0)
	hosts = append(hosts, t.MinIOServerHost())
	hosts = append(hosts, "*."+t.MinIOHeadlessServiceHost())
	return hosts
}

// MinIOServerHost returns ClusterIP service Host for current Tenant
func (t *Tenant) MinIOServerHost() string {
	return fmt.Sprintf("%s.%s.svc.%s", t.MinIOCIServiceName(), t.Namespace, ClusterDomain)
}

// ConsoleServerHost returns ClusterIP service Host for current Console Tenant
func (t *Tenant) ConsoleServerHost() string {
	return fmt.Sprintf("%s.%s.svc.%s", t.ConsoleCIServiceName(), t.Namespace, ClusterDomain)
}

// MinIOHeadlessServiceHost returns headless service Host for current Tenant
func (t *Tenant) MinIOHeadlessServiceHost() string {
	if t.Spec.Zones[0].Servers == 1 {
		msg := "Please set the server count > 1"
		klog.V(2).Infof(msg)
		return ""
	}
	return fmt.Sprintf("%s.%s.svc.%s", t.MinIOHLServiceName(), t.Namespace, ClusterDomain)
}

// KESHosts returns the host names created for current KES StatefulSet
func (t *Tenant) KESHosts() []string {
	hosts := make([]string, 0)
	var i int32 = 0
	for i < t.Spec.KES.Replicas {
		hosts = append(hosts, fmt.Sprintf("%s-"+strconv.Itoa(int(i))+".%s.%s.svc.%s", t.KESStatefulSetName(), t.KESHLServiceName(), t.Namespace, ClusterDomain))
		i++
	}
	hosts = append(hosts, t.KESServiceHost())
	return hosts
}

// KESServiceEndpoint similar to KESServiceHost but a URL with current scheme
func (t *Tenant) KESServiceEndpoint() string {
	scheme := "http"
	if t.TLS() {
		scheme = "https"
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(t.KESServiceHost(), strconv.Itoa(miniov2.KESPort)),
	}
	return u.String()
}

// KESServiceHost returns headless service Host for KES in current Tenant
func (t *Tenant) KESServiceHost() string {
	return fmt.Sprintf("%s.%s.svc.%s", t.KESHLServiceName(), t.Namespace, ClusterDomain)
}

// S3BucketDNS indicates if Bucket DNS feature is enabled.
func (t *Tenant) S3BucketDNS() bool {
	return t.Spec.S3 != nil && t.Spec.S3.BucketDNS
}

// HasKESEnabled checks if kes configuration is provided by user
func (t *Tenant) HasKESEnabled() bool {
	return t.Spec.KES != nil
}

// HasConsoleEnabled checks if the console has been enabled by the user
func (t *Tenant) HasConsoleEnabled() bool {
	return t.Spec.Console != nil
}

// HasConsoleSecret returns true if the user has provided an console secret
// for a Tenant else false
func (t *Tenant) HasConsoleSecret() bool {
	return t.Spec.Console != nil && t.Spec.Console.ConsoleSecret != nil
}

// UpdateURL returns the URL for the sha256sum location of the new binary
func (t *Tenant) UpdateURL(lrTime time.Time, overrideURL string) (string, error) {
	if overrideURL == "" {
		overrideURL = miniov2.DefaultMinIOUpdateURL
	}
	u, err := url.Parse(overrideURL)
	if err != nil {
		return "", err
	}
	u.Path = u.Path + "/minio." + releasePrefix + "." + lrTime.Format(minioReleaseTagTimeLayout) + ".sha256sum"
	return u.String(), nil
}

// MinIOServerHostAddress similar to MinIOServerHost but returns host with port
func (t *Tenant) MinIOServerHostAddress() string {
	var port int

	if t.TLS() {
		port = miniov2.MinIOTLSPortLoadBalancerSVC
	} else {
		port = miniov2.MinIOPortLoadBalancerSVC
	}

	return net.JoinHostPort(t.MinIOServerHost(), strconv.Itoa(port))
}

// MinIOServerEndpoint similar to MinIOServerHostAddress but a URL with current scheme
func (t *Tenant) MinIOServerEndpoint() string {
	scheme := "http"
	if t.TLS() {
		scheme = "https"
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   t.MinIOServerHostAddress(),
	}
	return u.String()
}

// MinIOHealthCheck check MinIO cluster health
func (t *Tenant) MinIOHealthCheck() bool {
	// Keep TLS config.
	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // FIXME: use trusted CA
	}

	req, err := http.NewRequest(http.MethodGet, t.MinIOServerEndpoint()+"/minio/health/cluster", nil)
	if err != nil {
		return false
	}

	httpClient := &http.Client{
		Transport:
		// For more details about various values used here refer
		// https://golang.org/pkg/net/http/#Transport documentation
		&http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   2 * time.Second,
				KeepAlive: 10 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
			TLSClientConfig:       tlsConfig,
			// Go net/http automatically unzip if content-type is
			// gzip disable this feature, as we are always interested
			// in raw stream.
			DisableCompression: true,
		},
	}
	defer httpClient.CloseIdleConnections()

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}

	return resp.StatusCode == http.StatusOK
}

// NewMinIOAdmin initializes a new madmin.Client for operator interaction
func (t *Tenant) NewMinIOAdmin(minioSecret map[string][]byte) (*madmin.AdminClient, error) {
	host := t.MinIOServerHostAddress()
	if host == "" {
		return nil, errors.New("MinIO server host is empty")
	}

	accessKey, ok := minioSecret["accesskey"]
	if !ok {
		return nil, errors.New("MinIO server accesskey not set")
	}

	secretKey, ok := minioSecret["secretkey"]
	if !ok {
		return nil, errors.New("MinIO server secretkey not set")
	}

	opts := &madmin.Options{
		Secure: t.TLS(),
		Creds:  credentials.NewStaticV4(string(accessKey), string(secretKey), ""),
	}

	madmClnt, err := madmin.NewWithOptions(host, opts)
	if err != nil {
		return nil, err
	}

	if opts.Secure {
		// FIXME: add trusted CA
		madmClnt = setUpInsecureTLS(madmClnt)
	}

	return madmClnt, nil
}

// CreateConsoleUser function creates an admin users
func (t *Tenant) CreateConsoleUser(madmClnt *madmin.AdminClient, userCredentialSecrets []*corev1.Secret, skipCreateUser bool) error {
	// add user with a 20 seconds timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	for _, secret := range userCredentialSecrets {
		consoleAccessKey, ok := secret.Data["CONSOLE_ACCESS_KEY"]
		if !ok {
			return errors.New("CONSOLE_ACCESS_KEY not provided")
		}
		// skipCreateUser handles the scenario of LDAP users that are not created in MinIO but still need to have a policy assigned
		if !skipCreateUser {
			consoleSecretKey, ok := secret.Data["CONSOLE_SECRET_KEY"]
			if !ok {
				return errors.New("CONSOLE_SECRET_KEY not provided")
			}
			if err := madmClnt.AddUser(ctx, string(consoleAccessKey), string(consoleSecretKey)); err != nil {
				return err
			}
		}
		if err := madmClnt.SetPolicy(context.Background(), miniov2.ConsoleAdminPolicyName, string(consoleAccessKey), false); err != nil {
			return err
		}
	}
	return nil
}

// Validate validate single zone as per MinIO deployment requirements
func (z *Zone) Validate(zi int) error {
	// Make sure the replicas are not 0 on any zone
	if z.Servers <= 0 {
		return fmt.Errorf("zone #%d cannot have 0 servers", zi)
	}

	// Make sure the zones don't have 0 volumes
	if z.VolumesPerServer <= 0 {
		return fmt.Errorf("zone #%d cannot have 0 volumes per server", zi)
	}

	if z.Servers*z.VolumesPerServer < 4 {
		// Erasure coding has few requirements.
		switch z.Servers {
		case 1:
			return fmt.Errorf("zone #%d setup must have a minimum of 4 volumes per server", zi)
		case 2:
			return fmt.Errorf("zone #%d setup must have a minimum of 2 volumes per server", zi)
		case 3:
			return fmt.Errorf("zone #%d setup must have a minimum of 2 volumes per server", zi)
		}
	}

	// Mandate a VolumeClaimTemplate
	if z.VolumeClaimTemplate == nil {
		return errors.New("a volume claim template must be specified")
	}

	// Mandate a resource request
	if z.VolumeClaimTemplate.Spec.Resources.Requests == nil {
		return errors.New("volume claim template must specify resource request")
	}

	// Mandate a request of storage
	if z.VolumeClaimTemplate.Spec.Resources.Requests.Storage() == nil {
		return errors.New("volume claim template must specify resource storage request")
	}

	// Make sure the storage request is not 0
	if z.VolumeClaimTemplate.Spec.Resources.Requests.Storage().Value() <= 0 {
		return errors.New("volume size must be greater than 0")
	}

	// Make sure access mode is provided
	if len(z.VolumeClaimTemplate.Spec.AccessModes) == 0 {
		return errors.New("volume access mode must be specified")
	}

	return nil
}

// Validate returns an error if any configuration of the MinIO Tenant is invalid
func (t *Tenant) Validate() error {
	if t.Spec.Zones == nil {
		return errors.New("zones must be configured")
	}

	if t.Spec.CredsSecret == nil {
		return errors.New("please set credsSecret secret with credentials for Tenant")
	}

	// Every zone must contain a Volume Claim Template
	for zi, zone := range t.Spec.Zones {
		if err := zone.Validate(zi); err != nil {
			return err
		}
	}

	return nil
}

// Set up admin client to use self certificates
func setUpInsecureTLS(api *madmin.AdminClient) *madmin.AdminClient {
	// Keep TLS config.
	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	var transport http.RoundTripper = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		TLSClientConfig: tlsConfig,
	}

	// Set custom transport.
	api.SetCustomTransport(transport)
	return api
}

// OwnerRef returns the OwnerReference to be added to all resources created by Tenant
func (t *Tenant) OwnerRef() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(t, schema.GroupVersionKind{
			Group:   SchemeGroupVersion.Group,
			Version: SchemeGroupVersion.Version,
			Kind:    miniov2.MinIOCRDResourceKind,
		}),
	}
}

// TLS indicates whether TLS is enabled for this tenant
func (t *Tenant) TLS() bool {
	return t.AutoCert() || t.ExternalCert()
}
