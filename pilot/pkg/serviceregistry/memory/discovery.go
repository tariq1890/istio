// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"fmt"
	"net"
	"time"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/spiffe"
)

var (
	// PortHTTPName is the HTTP port name
	PortHTTPName = "http"
)

// NewDiscovery builds a memory ServiceDiscovery
func NewDiscovery(services map[config.Hostname]*model.Service, versions int) *ServiceDiscovery {
	return &ServiceDiscovery{
		services: services,
		versions: versions,
	}
}

// MakeService creates a memory service
func MakeService(hostname config.Hostname, address string) *model.Service {
	return &model.Service{
		CreationTime: time.Now(),
		Hostname:     hostname,
		Address:      address,
		Ports: []*model.Port{
			{
				Name:     PortHTTPName,
				Port:     80, // target port 80
				Protocol: config.ProtocolHTTP,
			}, {
				Name:     "http-status",
				Port:     81, // target port 1081
				Protocol: config.ProtocolHTTP,
			}, {
				Name:     "custom",
				Port:     90, // target port 1090
				Protocol: config.ProtocolTCP,
			}, {
				Name:     "mongo",
				Port:     100, // target port 1100
				Protocol: config.ProtocolMongo,
			}, {
				Name:     "redis",
				Port:     110, // target port 1110
				Protocol: config.ProtocolRedis,
			}, {
				Name:     "mysql",
				Port:     120, // target port 1120
				Protocol: config.ProtocolMySQL,
			},
		},
	}
}

// MakeExternalHTTPService creates memory external service
func MakeExternalHTTPService(hostname config.Hostname, isMeshExternal bool, address string) *model.Service {
	return &model.Service{
		CreationTime: time.Now(),
		Hostname:     hostname,
		Address:      address,
		MeshExternal: isMeshExternal,
		Ports: []*model.Port{{
			Name:     "http",
			Port:     80,
			Protocol: config.ProtocolHTTP,
		}},
	}
}

// MakeExternalHTTPSService creates memory external service
func MakeExternalHTTPSService(hostname config.Hostname, isMeshExternal bool, address string) *model.Service {
	return &model.Service{
		CreationTime: time.Now(),
		Hostname:     hostname,
		Address:      address,
		MeshExternal: isMeshExternal,
		Ports: []*model.Port{{
			Name:     "https",
			Port:     443,
			Protocol: config.ProtocolHTTPS,
		}},
	}
}

// MakeInstance creates a memory instance, version enumerates endpoints
func MakeInstance(service *model.Service, port *model.Port, version int, az string) *model.ServiceInstance {
	if service.External() {
		return nil
	}

	// we make port 80 same as endpoint port, otherwise, it's distinct
	target := port.Port
	if target != 80 {
		target += 1000
	}

	return &model.ServiceInstance{
		Endpoint: model.NetworkEndpoint{
			Address:     MakeIP(service, version),
			Port:        target,
			ServicePort: port,
			Locality:    az,
		},
		Service: service,
		Labels:  map[string]string{"version": fmt.Sprintf("v%d", version)},
	}
}

// MakeIP creates a fake IP address for a service and instance version
func MakeIP(service *model.Service, version int) string {
	// external services have no instances
	if service.External() {
		return ""
	}
	ip := net.ParseIP(service.Address).To4()
	ip[2] = byte(1)
	ip[3] = byte(version)
	return ip.String()
}

// ServiceDiscovery is a memory discovery interface
type ServiceDiscovery struct {
	services                      map[config.Hostname]*model.Service
	versions                      int
	WantGetProxyServiceInstances  []*model.ServiceInstance
	ServicesError                 error
	GetServiceError               error
	InstancesError                error
	GetProxyServiceInstancesError error
}

// ClearErrors clear errors used for failures during model.ServiceDiscovery interface methods
func (sd *ServiceDiscovery) ClearErrors() {
	sd.ServicesError = nil
	sd.GetServiceError = nil
	sd.InstancesError = nil
	sd.GetProxyServiceInstancesError = nil
}

// AddService will add to the registry the provided service
func (sd *ServiceDiscovery) AddService(name config.Hostname, svc *model.Service) {
	sd.services[name] = svc
}

// Services implements discovery interface
func (sd *ServiceDiscovery) Services() ([]*model.Service, error) {
	if sd.ServicesError != nil {
		return nil, sd.ServicesError
	}
	out := make([]*model.Service, 0, len(sd.services))
	for _, service := range sd.services {
		out = append(out, service)
	}
	return out, sd.ServicesError
}

// GetService implements discovery interface
func (sd *ServiceDiscovery) GetService(hostname config.Hostname) (*model.Service, error) {
	if sd.GetServiceError != nil {
		return nil, sd.GetServiceError
	}
	val := sd.services[hostname]
	return val, sd.GetServiceError
}

// InstancesByPort implements discovery interface
func (sd *ServiceDiscovery) InstancesByPort(hostname config.Hostname, num int,
	labels config.LabelsCollection) ([]*model.ServiceInstance, error) {
	if sd.InstancesError != nil {
		return nil, sd.InstancesError
	}
	service, ok := sd.services[hostname]
	if !ok {
		return nil, sd.InstancesError
	}
	out := make([]*model.ServiceInstance, 0)
	if service.External() {
		return out, sd.InstancesError
	}
	if port, ok := service.Ports.GetByPort(num); ok {
		for v := 0; v < sd.versions; v++ {
			if labels.HasSubsetOf(map[string]string{"version": fmt.Sprintf("v%d", v)}) {
				out = append(out, MakeInstance(service, port, v, "zone/region"))
			}
		}
	}
	return out, sd.InstancesError
}

// GetProxyServiceInstances implements discovery interface
func (sd *ServiceDiscovery) GetProxyServiceInstances(node *model.Proxy) ([]*model.ServiceInstance, error) {
	if sd.GetProxyServiceInstancesError != nil {
		return nil, sd.GetProxyServiceInstancesError
	}
	if sd.WantGetProxyServiceInstances != nil {
		return sd.WantGetProxyServiceInstances, nil
	}
	out := make([]*model.ServiceInstance, 0)
	for _, service := range sd.services {
		if !service.External() {
			for v := 0; v < sd.versions; v++ {
				// Only one IP for memory discovery?
				if node.IPAddresses[0] == MakeIP(service, v) {
					for _, port := range service.Ports {
						out = append(out, MakeInstance(service, port, v, "region/zone"))
					}
				}
			}

		}
	}
	return out, sd.GetProxyServiceInstancesError
}

func (sd *ServiceDiscovery) GetProxyWorkloadLabels(proxy *model.Proxy) (config.LabelsCollection, error) {
	if sd.GetProxyServiceInstancesError != nil {
		return nil, sd.GetProxyServiceInstancesError
	}
	// no useful labels from the ServiceInstances created by MakeInstance()
	return nil, nil
}

// ManagementPorts implements discovery interface
func (sd *ServiceDiscovery) ManagementPorts(addr string) model.PortList {
	return model.PortList{{
		Name:     "http",
		Port:     3333,
		Protocol: config.ProtocolHTTP,
	}, {
		Name:     "custom",
		Port:     9999,
		Protocol: config.ProtocolTCP,
	}}
}

// WorkloadHealthCheckInfo implements discovery interface
func (sd *ServiceDiscovery) WorkloadHealthCheckInfo(addr string) model.ProbeList {
	return nil
}

// GetIstioServiceAccounts gets the Istio service accounts for a service hostname.
func (sd *ServiceDiscovery) GetIstioServiceAccounts(hostname config.Hostname, ports []int) []string {
	if hostname == "world.default.svc.cluster.local" {
		return []string{
			spiffe.MustGenSpiffeURI("default", "serviceaccount1"),
			spiffe.MustGenSpiffeURI("default", "serviceaccount2"),
		}
	}
	return make([]string, 0)
}
