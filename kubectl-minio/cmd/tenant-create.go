/*
 * This file is part of MinIO Operator
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

package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/kubectl-minio/cmd/helpers"
	"github.com/minio/kubectl-minio/cmd/resources"
	"github.com/minio/minio/pkg/color"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"
	operatorv1 "github.com/minio/operator/pkg/client/clientset/versioned"
	"github.com/spf13/cobra"
)

const (
	createDesc = `
'create' command creates a new MinIO tenant`
	createExample       = `  kubectl minio tenant create --name tenant1 --servers 4 --volumes 16 --capacity 16Ti --namespace tenant1-ns`
	tenantSecretSuffix  = "-creds-secret"
	consoleSecretSuffix = "-console-secret"
)

type createCmd struct {
	out        io.Writer
	errOut     io.Writer
	output     bool
	tenantOpts resources.TenantOptions
}

func newTenantCreateCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	c := &createCmd{out: out, errOut: errOut}

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a MinIO tenant",
		Long:    createDesc,
		Example: createExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.validate(); err != nil {
				return err
			}
			return c.run(args)
		},
	}

	f := cmd.Flags()
	f.StringVar(&c.tenantOpts.Name, "name", "", "name of the MinIO tenant to create")
	f.Int32Var(&c.tenantOpts.Servers, "servers", 0, "total number of pods in MinIO tenant")
	f.Int32Var(&c.tenantOpts.Volumes, "volumes", 0, "total number of volumes in the MinIO tenant")
	f.StringVar(&c.tenantOpts.Capacity, "capacity", "", "total raw capacity of MinIO tenant in this pool, e.g. 16Ti")
	f.StringVarP(&c.tenantOpts.NS, "namespace", "n", helpers.DefaultNamespace, "namespace scope for this request")
	f.StringVarP(&c.tenantOpts.StorageClass, "storage-class", "s", helpers.DefaultStorageclass, "storage class for this MinIO tenant")
	f.StringVar(&c.tenantOpts.KmsSecret, "kes-config", "", "name of secret with details for enabling encryption, refer example https://github.com/minio/operator/blob/master/examples/kes-secret.yaml")
	f.BoolVarP(&c.output, "output", "o", false, "dry run this command and generate requisite yaml")

	return cmd
}

func (c *createCmd) validate() error {
	c.tenantOpts.SecretName = c.tenantOpts.Name + tenantSecretSuffix
	c.tenantOpts.ConsoleSecret = c.tenantOpts.Name + consoleSecretSuffix
	c.tenantOpts.Image = helpers.DefaultTenantImage
	return c.tenantOpts.Validate()
}

// run initializes local config and installs MinIO Operator to Kubernetes cluster.
func (c *createCmd) run(args []string) error {
	// Create operator and kube client
	oclient, err := helpers.GetKubeOperatorClient()
	if err != nil {
		return err
	}
	kclient, err := helpers.GetKubeClient()
	if err != nil {
		return err
	}

	// generate the resources
	t, err := resources.NewTenant(&c.tenantOpts)
	if err != nil {
		return err
	}
	s := resources.NewSecretForTenant(&c.tenantOpts)
	console := resources.NewSecretForConsole(&c.tenantOpts)

	// create resources
	if !c.output {
		return createTenant(oclient, kclient, t, s, console)
	}
	ot, err := yaml.Marshal(&t)
	if err != nil {
		return err
	}
	os, err := yaml.Marshal(&s)
	if err != nil {
		return err
	}
	oc, err := yaml.Marshal(&console)
	if err != nil {
		return err
	}
	fmt.Println(string(ot))
	fmt.Println("---")
	fmt.Println(string(os))
	fmt.Println("---")
	fmt.Println(string(oc))
	return nil
}

func createTenant(oclient *operatorv1.Clientset, kclient *kubernetes.Clientset, t *miniov1.Tenant, s, console *corev1.Secret) error {
	if _, err := kclient.CoreV1().Secrets(t.Namespace).Create(context.Background(), s, metav1.CreateOptions{}); err != nil {
		return err
	}
	if _, err := kclient.CoreV1().Secrets(t.Namespace).Create(context.Background(), console, metav1.CreateOptions{}); err != nil {
		return err
	}
	to, err := oclient.MinioV1().Tenants(t.Namespace).Create(context.Background(), t, v1.CreateOptions{})
	if err != nil {
		return err
	}
	if color.IsTerminal() {
		consolePort := miniov1.ConsolePort
		minioPort := miniov1.MinIOPort
		if to.HasCertConfig() || to.AutoCert() {
			consolePort = miniov1.ConsoleTLSPort
		}
		printBanner(to.ObjectMeta.Name, string(console.Data["CONSOLE_ACCESS_KEY"]), string(console.Data["CONSOLE_SECRET_KEY"]),
			to.ConsoleCIServiceName(), to.MinIOHLServiceName(), consolePort, minioPort, (to.HasCertConfig() || to.AutoCert()))
	}
	return nil
}

func printBanner(tenantName, user, pwd, consoleSVCName, minioSVCName string, consolePort, minioPort int, tls bool) {
	minioLocalPort := 9000
	consoleLocalPort := 9090
	scheme := "http"
	if tls {
		scheme = "https"
	}
	fmt.Printf(color.Bold(fmt.Sprintf("\nMinIO Tenant '%s' created\n\n", tenantName)))
	fmt.Printf(color.Blue("Username: ") + color.Bold(fmt.Sprintf("%s \n", user)))
	fmt.Printf(color.Blue("Password: ") + color.Bold(fmt.Sprintf("%s \n\n", pwd)))

	fmt.Printf(color.Blue("Web interface access: \n"))
	fmt.Printf(fmt.Sprintf("\t$ kubectl port-forward svc/%s %d:%d\n", consoleSVCName, consoleLocalPort, consolePort))
	fmt.Printf(fmt.Sprintf("\tPoint browser to %s://localhost:%d\n\n", scheme, consoleLocalPort))

	fmt.Printf(color.Blue("Object storage access: \n"))
	fmt.Printf(fmt.Sprintf("\t$ kubectl port-forward svc/%s %d:%d\n", minioSVCName, minioLocalPort, minioPort))
	fmt.Printf((fmt.Sprintf("\t$ mc alias set %s %s://localhost:%d %s %s\n\n", tenantName, scheme, minioLocalPort, user, pwd)))
}
