//
// DISCLAIMER
//
// Copyright 2019 ArangoDB Inc, Cologne, Germany
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
// Author Max Neunhoeffer
//

package resources

import (
	"github.com/arangodb/kube-arangodb/pkg/util/k8sutil"

	coreosv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	clientv1 "github.com/coreos/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func LabelsForExporterServiceMonitor(deploymentName string) map[string]string {
	return map[string]string{
		k8sutil.LabelKeyArangoDeployment: deploymentName,
		k8sutil.LabelKeyApp:              k8sutil.AppName,
		"context":                        "metrics",
	}
}

func LabelsForExporterServiceMonitorSelector(deploymentName string) map[string]string {
	return map[string]string{
		k8sutil.LabelKeyArangoDeployment: deploymentName,
		k8sutil.LabelKeyApp:              k8sutil.AppName,
	}
}

// EnsureMonitoringClient returns a client for looking at ServiceMonitors
// and keeps it in the Resources.
func (r *Resources) EnsureMonitoringClient() (*clientv1.MonitoringV1Client, error) {
	if r.monitoringClient != nil {
		return r.monitoringClient, nil
	}

	// Make a client:
	var restConfig *rest.Config
	restConfig, err := k8sutil.InClusterConfig()
	if err != nil {
		return nil, maskAny(err)
	}
	mClient, err := clientv1.NewForConfig(restConfig)
	if err != nil {
		return nil, maskAny(err)
	}
	r.monitoringClient = mClient
	return mClient, nil
}

func (r *Resources) makeEndpoint(isSecure bool) coreosv1.Endpoint {
	if isSecure {
		kubecli := r.context.GetKubeCli()
		ns := r.context.GetNamespace()
		secrets := k8sutil.NewSecretCache(kubecli.CoreV1().Secrets(ns))
		spec := r.context.GetSpec()
		secretName := spec.TLS.GetCASecretName()
		cert, _, _, err := k8sutil.GetCASecret(secrets, secretName, nil)

		var tlsconfig *coreosv1.TLSConfig
		if err == nil {
			tlsconfig = &coreosv1.TLSConfig{
				CAFile:             cert,
				InsecureSkipVerify: false,
			}
		} else {
			tlsconfig = &coreosv1.TLSConfig{
				InsecureSkipVerify: true,
			}
		}
		return coreosv1.Endpoint{
			Port:      "exporter",
			Interval:  "10s",
			Scheme:    "https",
			TLSConfig: tlsconfig,
		}
	} else {
		return coreosv1.Endpoint{
			Port:     "exporter",
			Interval: "10s",
			Scheme:   "http",
		}
	}
}

// EnsureServiceMonitor creates or updates a ServiceMonitor.
func (r *Resources) EnsureServiceMonitor() error {
	// Some preparations:
	log := r.log
	apiObject := r.context.GetAPIObject()
	deploymentName := apiObject.GetName()
	ns := apiObject.GetNamespace()
	owner := apiObject.AsOwner()
	spec := r.context.GetSpec()
	wantMetrics := spec.Metrics.IsEnabled()
	serviceMonitorName := k8sutil.CreateExporterClientServiceName(deploymentName)

	mClient, err := r.EnsureMonitoringClient()
	if err != nil {
		log.Error().Err(err).Msgf("Cannot get a monitoring client.")
		return maskAny(err)
	}

	// Check if ServiceMonitor already exists
	serviceMonitors := mClient.ServiceMonitors(ns)
	_, err = serviceMonitors.Get(serviceMonitorName, metav1.GetOptions{})
	if err != nil {
		if k8sutil.IsNotFound(err) {
			if !wantMetrics {
				return nil
			}
			// Need to create one:
			smon := &coreosv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:            serviceMonitorName,
					Labels:          LabelsForExporterServiceMonitor(deploymentName),
					OwnerReferences: []metav1.OwnerReference{owner},
				},
				Spec: coreosv1.ServiceMonitorSpec{
					JobLabel: "k8s-app",
					Endpoints: []coreosv1.Endpoint{
						r.makeEndpoint(spec.IsSecure()),
					},
					Selector: metav1.LabelSelector{
						MatchLabels: LabelsForExporterServiceMonitorSelector(deploymentName),
					},
				},
			}
			smon, err = serviceMonitors.Create(smon)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to create ServiceMonitor %s", serviceMonitorName)
				return maskAny(err)
			}
			log.Debug().Msgf("ServiceMonitor %s successfully created.", serviceMonitorName)
			return nil
		} else {
			log.Error().Err(err).Msgf("Failed to get ServiceMonitor %s", serviceMonitorName)
			return maskAny(err)
		}
	}
	if wantMetrics {
		log.Debug().Msgf("ServiceMonitor %s already found, no need to create.",
			serviceMonitorName)
		return nil
	}
	// Need to get rid of the ServiceMonitor:
	err = serviceMonitors.Delete(serviceMonitorName, &metav1.DeleteOptions{})
	if err == nil {
		log.Debug().Msgf("Deleted ServiceMonitor %s", serviceMonitorName)
		return nil
	}
	log.Error().Err(err).Msgf("Could not delete ServiceMonitor %s.", serviceMonitorName)
	return maskAny(err)
}
