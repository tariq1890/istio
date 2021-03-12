// Copyright Istio Authors
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

package gateway

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"
	svc "sigs.k8s.io/gateway-api/apis/v1alpha1"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/memory"
	controller2 "istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/gvk"
)

var (
	gatewayClassSpec = &svc.GatewayClassSpec{
		Controller: ControllerName,
	}
	gatewaySpec = &svc.GatewaySpec{
		GatewayClassName: "gwclass",
		Listeners: []svc.Listener{
			{
				Port:     9009,
				Protocol: "HTTP",
				Routes: svc.RouteBindingSelector{
					Namespaces: svc.RouteNamespaces{From: svc.RouteSelectAll},
					Group:      gvk.HTTPRoute.Group,
					Kind:       gvk.HTTPRoute.Kind,
				},
			},
		},
	}
	httpRouteSpec = &svc.HTTPRouteSpec{
		Gateways:  svc.RouteGateways{Allow: svc.GatewayAllowAll},
		Hostnames: []svc.Hostname{"test.cluster.local"},
	}

	expectedgw = &networking.Gateway{
		Servers: []*networking.Server{
			{
				Port: &networking.Port{
					Number:   9009,
					Name:     "http-9009-gateway-gwspec-ns1",
					Protocol: "HTTP",
				},
				Hosts: []string{"*"},
			},
		},
		Selector: map[string]string{
			"istio": "ingressgateway",
		},
	}

	expectedvs = &networking.VirtualService{
		Hosts: []string{
			"test.cluster.local",
		},
		Gateways: []string{
			"ns1/gwspec-istio-autogenerated-k8s-gateway",
		},
		Http: []*networking.HTTPRoute{},
	}
)

func TestListInvalidGroupVersionKind(t *testing.T) {
	g := NewWithT(t)
	clientSet := fake.NewSimpleClientset()
	store := memory.NewController(memory.Make(collections.All))
	controller := NewController(clientSet, store, controller2.Options{})

	typ := config.GroupVersionKind{Kind: "wrong-kind"}
	c, err := controller.List(typ, "ns1")
	g.Expect(c).To(HaveLen(0))
	g.Expect(err).To(HaveOccurred())
}

func TestListGatewayResourceType(t *testing.T) {
	g := NewWithT(t)

	clientSet := fake.NewSimpleClientset()
	store := memory.NewController(memory.Make(collections.All))
	controller := NewController(clientSet, store, controller2.Options{})

	gwClassType := collections.K8SServiceApisV1Alpha1Gatewayclasses.Resource()
	gwSpecType := collections.K8SServiceApisV1Alpha1Gateways.Resource()
	k8sHTTPRouteType := collections.K8SServiceApisV1Alpha1Httproutes.Resource()

	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gwClassType.GroupVersionKind(),
			Name:             "gwclass",
			Namespace:        "ns1",
		},
		Spec: gatewayClassSpec,
	})
	if _, err := store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gwSpecType.GroupVersionKind(),
			Name:             "gwspec",
			Namespace:        "ns1",
		},
		Spec: gatewaySpec,
	}); err != nil {
		t.Fatal(err)
	}
	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: k8sHTTPRouteType.GroupVersionKind(),
			Name:             "http-route",
			Namespace:        "ns1",
		},
		Spec: httpRouteSpec,
	})

	cfg, err := controller.List(gvk.Gateway, "ns1")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg).To(HaveLen(1))
	for _, c := range cfg {
		g.Expect(c.GroupVersionKind).To(Equal(gvk.Gateway))
		g.Expect(c.Name).To(Equal("gwspec" + "-" + constants.KubernetesGatewayName))
		g.Expect(c.Namespace).To(Equal("ns1"))
		g.Expect(c.Spec).To(Equal(expectedgw))
	}
}

func TestListVirtualServiceResourceType(t *testing.T) {
	g := NewWithT(t)

	clientSet := fake.NewSimpleClientset()
	store := memory.NewController(memory.Make(collections.All))
	controller := NewController(clientSet, store, controller2.Options{})

	gwClassType := collections.K8SServiceApisV1Alpha1Gatewayclasses.Resource()
	gwSpecType := collections.K8SServiceApisV1Alpha1Gateways.Resource()
	k8sHTTPRouteType := collections.K8SServiceApisV1Alpha1Httproutes.Resource()

	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gwClassType.GroupVersionKind(),
			Name:             "gwclass",
			Namespace:        "ns1",
		},
		Spec: gatewayClassSpec,
	})
	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gwSpecType.GroupVersionKind(),
			Name:             "gwspec",
			Namespace:        "ns1",
		},
		Spec: gatewaySpec,
	})
	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: k8sHTTPRouteType.GroupVersionKind(),
			Name:             "http-route",
			Namespace:        "ns1",
		},
		Spec: httpRouteSpec,
	})

	cfg, err := controller.List(gvk.VirtualService, "ns1")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg).To(HaveLen(1))
	for _, c := range cfg {
		g.Expect(c.GroupVersionKind).To(Equal(gvk.VirtualService))
		g.Expect(c.Name).To(Equal("http-route-" + constants.KubernetesGatewayName))
		g.Expect(c.Namespace).To(Equal("ns1"))
		g.Expect(c.Spec).To(Equal(expectedvs))
	}
}
