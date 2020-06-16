// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kind

import (
	"context"
	"github.com/pkg/errors"
	k8sProvider "github.com/prometheus/test-infra/pkg/provider/k8s"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"

	//clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/kind/pkg/cmd"
	"strings"
	//"encoding/base64"
	//"encoding/json"
	"fmt"
	//"io/ioutil"
	"log"
	//"os"
	//"regexp"
	//"strings"

	"github.com/prometheus/test-infra/pkg/provider"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/kind/pkg/cluster"
	//get "sigs.k8s.io/kind/pkg/cmd/kind/get/clusters"
)

// New is the KIND constructor.

type Resource = provider.Resource

// GKE holds the fields used to generate an API request.
type KIND struct {

	// The k8s provider used when we work with the manifest files.
	k8sProvider *k8sProvider.K8s
	kindProvider *cluster.Provider
	// DeploymentFiles files provided from the cli.
	DeploymentFiles []string
	// Variables to substitute in the DeploymentFiles.
	// These are also used when the command requires some variables that are not provided by the deployment file.
	DeploymentVars map[string]string
	// Content bytes after parsing the template variables, grouped by filename.
	kindResources []Resource
	// K8s resource.runtime objects after parsing the template variables, grouped by filename.
	k8sResources []k8sProvider.Resource

	ctx context.Context
}

func New() *KIND {

	return &KIND{
		DeploymentVars: make(map[string]string),
		kindProvider: cluster.NewProvider(
			cluster.ProviderWithLogger(cmd.NewLogger()),
		),
		ctx: context.Background(),

	}
}

// GKEDeploymentsParse parses the cluster/nodepool deployment files and saves the result as bytes grouped by the filename.
// Any variables passed to the cli will be replaced in the resources files following the golang text template format.
func (c *KIND) KINDDeploymentsParse(*kingpin.ParseContext) error {
	//c.setProjectID()
	//
	deploymentResource, err := provider.DeploymentsParse(c.DeploymentFiles, c.DeploymentVars)
	if err != nil {
		log.Fatalf("Couldn't parse deployment files: %v", err)
	}
	fmt.Println(deploymentResource)
	c.kindResources = deploymentResource
	return nil
}

func (c *KIND) K8SDeploymentsParse(*kingpin.ParseContext) error {

	deploymentResource, err := provider.DeploymentsParse(c.DeploymentFiles, c.DeploymentVars)
	if err != nil {
		log.Fatalf("Couldn't parse deployment files: %v", err)
	}

	for _, deployment := range deploymentResource {

		decode := scheme.Codecs.UniversalDeserializer().Decode
		k8sObjects := make([]runtime.Object, 0)

		for _, text := range strings.Split(string(deployment.Content), provider.Separator) {
			text = strings.TrimSpace(text)
			if len(text) == 0 {
				continue
			}

			resource, _, err := decode([]byte(text), nil, nil)
			if err != nil {
				return errors.Wrapf(err, "decoding the resource file:%v, section:%v...", deployment.FileName, text[:100])
			}
			if resource == nil {
				continue
			}
			k8sObjects = append(k8sObjects, resource)
		}
		if len(k8sObjects) > 0 {
			c.k8sResources = append(c.k8sResources, k8sProvider.Resource{FileName: deployment.FileName, Objects: k8sObjects})
		}
	}
	return nil
}


// ClusterCreate create a new cluster or applies changes to an existing cluster.
func (c *KIND) ClusterCreate(*kingpin.ParseContext) error {

	clusterName, ok := c.DeploymentVars["CLUSTER_NAME"]
	if !ok {
		return fmt.Errorf("missing required CLUSTER_NAME variable")
	}

	for _, deployment := range c.DeploymentFiles {
		CreateWithConfigFile := cluster.CreateWithConfigFile(deployment)

		err := c.kindProvider.Create(clusterName, CreateWithConfigFile)
		if err != nil {
			log.Fatalf("creating cluster err:%v", err)
		}
	}

	return nil
}

// ClusterDelete deletes a k8s cluster.
func (c *KIND) ClusterDelete(*kingpin.ParseContext) error {
	clusterName, ok := c.DeploymentVars["CLUSTER_NAME"]
	if !ok {
		return fmt.Errorf("missing required CLUSTER_NAME variable")
	}

	err := c.kindProvider.Delete(clusterName, "/home/raj/.kube/config")
	if err != nil {
		log.Fatalf("creating cluster err:%v", err)
	}
	return nil
}

// NewK8sProvider sets the k8s provider used for deploying k8s manifests.
func (c *KIND) NewK8sProvider(*kingpin.ParseContext) error {

	var err error
	apiConfig, err := clientcmd.LoadFromFile("/home/raj/.kube/config")
	if err != nil {
		fmt.Errorf("failed to load user provided kubeconfig: %v", err)
	}
	c.k8sProvider, err = k8sProvider.New(c.ctx, apiConfig)
	if err != nil {
		log.Fatal("k8s provider error", err)
	}
	return nil

}

func (c *KIND) ResourceApply(*kingpin.ParseContext) error {
	//fmt.Println(c.k8sResources)
	if err := c.k8sProvider.ResourceApply(c.k8sResources); err != nil {
		log.Fatal("error while applying a resource err:", err)
	}
	return nil
}
//
//// ResourceDelete calls k8s.ResourceDelete to apply the k8s objects in the manifest files.
func (c *KIND) ResourceDelete(*kingpin.ParseContext) error {
	if err := c.k8sProvider.ResourceDelete(c.k8sResources); err != nil {
		log.Fatal("error while deleting objects from a manifest file err:", err)
	}
	return nil
}