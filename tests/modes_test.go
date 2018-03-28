//
// DISCLAIMER
//
// Copyright 2018 ArangoDB GmbH, Cologne, Germany
//
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
//
// Copyright holder is ArangoDB GmbH, Cologne, Germany
//
// Author Jan Christoph Uhde <jan@uhdejc.com>
//
package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dchest/uniuri"

	api "github.com/arangodb/kube-arangodb/pkg/apis/deployment/v1alpha"
	kubeArangoClient "github.com/arangodb/kube-arangodb/pkg/client"
	//"github.com/arangodb/kube-arangodb/pkg/util"
)

func TestProduction(t *testing.T) {
	subTest(t, api.DeploymentModeCluster, api.StorageEngineRocksDB)
}

func subTest(t *testing.T, mode api.DeploymentMode, engine api.StorageEngine) error {
	// check environment
	longOrSkip(t)

	// FIXME - add code
	// set production mode

	k8sNameSpace := getNamespace(t)
	k8sClient := mustNewKubeClient(t)
	deploymentClient := kubeArangoClient.MustNewInCluster()

	deploymentTemplate := newDeployment(strings.Replace(fmt.Sprintf("tu-%s-%s-%s", mode[:2], engine[:2], uniuri.NewLen(4)), ".", "", -1))
	deploymentTemplate.Spec.Mode = api.NewMode(mode)
	deploymentTemplate.Spec.StorageEngine = api.NewStorageEngine(engine)
	deploymentTemplate.Spec.TLS = api.TLSSpec{} // should auto-generate cert
	deploymentTemplate.Spec.Environment = api.NewEnvironment(api.EnvironmentProduction)
	deploymentTemplate.Spec.SetDefaults(deploymentTemplate.GetName()) // this must be last

	var dbserverCount int = *deploymentTemplate.Spec.DBServers.Count
	if dbserverCount < 3 {
		t.Fatalf("Not enough dbservers to run this test: server count %d", dbserverCount)
	}

	// Create deployment
	deployment, err := deploymentClient.DatabaseV1alpha().ArangoDeployments(k8sNameSpace).Create(deploymentTemplate)
	if err != nil {
		t.Fatalf("Create deployment failed: %v", err)
	}

	// FIXME - add code
	// check if it was possible to create a deployment in production the number of dbservers can not exceed the number of nodes

	// Wait for deployment to be ready
	deployment, err = waitUntilDeployment(deploymentClient, deploymentTemplate.GetName(), k8sNameSpace, deploymentIsReady())
	if err != nil {
		t.Fatalf("Deployment not running in time: %v", err)
	}

	// Create a database client
	ctx := context.Background()
	DBClient := mustNewArangodDatabaseClient(ctx, k8sClient, deployment, t)

	if err := waitUntilArangoDeploymentHealthy(deployment, DBClient, k8sClient, ""); err != nil {
		t.Fatalf("Deployment not healthy in time: %v", err)
	}

	// Cleanup
	removeDeployment(deploymentClient, deploymentTemplate.GetName(), k8sNameSpace)

	return nil
}
