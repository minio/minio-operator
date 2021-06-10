/*
 * Copyright (C) 2019, 2020, MinIO, Inc.
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
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/minio/operator/pkg/resources/statefulsets"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"k8s.io/klog/v2"

	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
)

func generateKESCryptoData(tenant *miniov2.Tenant) ([]byte, []byte, error) {
	privateKey, err := newPrivateKey(miniov2.DefaultEllipticCurve)
	if err != nil {
		klog.Errorf("Unexpected error during the ECDSA Key generation: %v", err)
		return nil, nil, err
	}

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		klog.Errorf("Unexpected error during encoding the ECDSA Private Key: %v", err)
		return nil, nil, err
	}

	var csrExtensions []pkix.Extension
	kesHosts := tenant.KESHosts()
	for _, host := range kesHosts {
		csrExtensions = append(csrExtensions, pkix.Extension{
			Id:       nil,
			Critical: false,
			Value:    []byte(host),
		})
	}

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("system:node:%s", tenant.KESWildCardName()),
			Organization: tenant.Spec.CertConfig.OrganizationName,
		},
		SignatureAlgorithm: x509.ECDSAWithSHA512,
		DNSNames:           tenant.KESHosts(),
		Extensions:         csrExtensions,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		klog.Errorf("Unexpected error during creating the CSR: %v", err)
		return nil, nil, err
	}
	return privKeyBytes, csrBytes, nil
}

// createKESCSR handles all the steps required to create the CSR: from creation of keys, submitting CSR and
// finally creating a secret that KES Statefulset will use to mount private key and certificate for TLS
// This Method Blocks till the CSR Request is approved via kubectl approve
func (c *Controller) createKESCSR(ctx context.Context, tenant *miniov2.Tenant) error {
	privKeysBytes, csrBytes, err := generateKESCryptoData(tenant)
	if err != nil {
		klog.Errorf("Private Key and CSR generation failed with error: %v", err)
		return err
	}

	err = c.createCertificateSigningRequest(ctx, tenant.KESPodLabels(), tenant.KESCSRName(), tenant.Namespace, csrBytes, tenant, "server")
	if err != nil {
		klog.Errorf("Unexpected error during the creation of the csr/%s: %v", tenant.KESCSRName(), err)
		return err
	}

	// fetch certificate from CSR
	certbytes, err := c.fetchCertificate(ctx, tenant.KESCSRName())
	if err != nil {
		klog.Errorf("Unexpected error during the creation of the csr/%s: %v", tenant.KESCSRName(), err)
		return err
	}

	// PEM encode private ECDSA key
	encodedPrivKey := pem.EncodeToMemory(&pem.Block{Type: privateKeyType, Bytes: privKeysBytes})

	// Create secret for KES Statefulset to use
	err = c.createSecret(ctx, tenant, tenant.KESPodLabels(), tenant.KESTLSSecretName(), encodedPrivKey, certbytes)
	if err != nil {
		klog.Errorf("Unexpected error during the creation of the secret/%s: %v", tenant.KESTLSSecretName(), err)
		return err
	}

	return nil
}

// kesStatefulSetMatchesSpec checks if the StatefulSet for KES matches what is expected and described from the Tenant
func kesStatefulSetMatchesSpec(tenant *miniov2.Tenant, kesStatefulSet *appsv1.StatefulSet) (bool, error) {
	if kesStatefulSet == nil {
		return false, errors.New("cannot process an empty kes StatefulSet")
	}
	if tenant == nil {
		return false, errors.New("cannot process an empty tenant")
	}
	// compare image directly
	if !tenant.Spec.KES.EqualImage(kesStatefulSet.Spec.Template.Spec.Containers[0].Image) {
		klog.V(2).Infof("Tenant %s KES version %s doesn't match: %s", tenant.Name,
			tenant.Spec.KES.Image, kesStatefulSet.Spec.Template.Spec.Containers[0].Image)
		return false, nil
	}
	// compare any other change from what is specified on the tenant
	expectedStatefulSet := statefulsets.NewForKES(tenant, tenant.KESHLServiceName())
	if !equality.Semantic.DeepDerivative(expectedStatefulSet.Spec, kesStatefulSet.Spec) {
		// some field set by the operator has changed
		return false, nil
	}
	return true, nil
}
