/*
MIT License

Copyright (c) 2022 Carlos Eduardo de Paula

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ExternalLoadBalancer{}, &ExternalLoadBalancerList{})
}

// User-side configuration for an external load balancer via CRDs

// ExternalLoadBalancer is the Schema for the externalloadbalancers API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:displayName="ExternalLoadBalancer Instance",resources={{ExternalLoadBalancer,lb.lbconfig.carlosedp.com/v1,externalloadbalancer},}
// +kubebuilder:resource:path="externalloadbalancers"
// +kubebuilder:resource:singular="externalloadbalancer"
// +kubebuilder:resource:shortName="elb"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="VIP",type="string",JSONPath=".spec.vip",description="Load Balancer VIP"
// +kubebuilder:printcolumn:name="Ports",type="string",JSONPath=".spec.ports",description="Load Balancer Ports"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.provider.vendor",description="Load Balancer Provider Backend"
// +kubebuilder:printcolumn:name="Nodes",type="string",JSONPath=".status.numnodes",description="Amount of nodes in the load balancer"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="Type of nodes in this Load Balancer"
// +kubebuilder:printcolumn:name="Matching Node Labels",type="string",JSONPath=".status.labels",description="Node Labels matching this Load Balancer"
type ExternalLoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalLoadBalancerSpec   `json:"spec,omitempty"`
	Status ExternalLoadBalancerStatus `json:"status,omitempty"`
}

// ExternalLoadBalancerSpec is the spec of a LoadBalancer instance.
type ExternalLoadBalancerSpec struct {
	// Vip is the Virtual IP configured in  this LoadBalancer instance
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=7
	// +kubebuilder:validation:MaxLength=15
	Vip string `json:"vip"`

	// Type is the node role type (master or infra) for the LoadBalancer instance
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=master;infra
	Type string `json:"type,omitempty"`

	// NodeLabels are the node labels used for router sharding as an alternative to "type". Optional.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	NodeLabels map[string]string `json:"nodelabels,omitempty"`

	// Ports is the ports exposed by this LoadBalancer instance
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=128
	Ports []int `json:"ports"`

	// Monitor is the path and port to monitor the LoadBalancer members
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	Monitor Monitor `json:"monitor"`

	// Provider is the LoadBalancer backend provider
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	Provider Provider `json:"provider"`
}

// Monitor defines a monitor object in the LoadBalancer.
type Monitor struct {
	// Name is the monitor name, it is set by the controller
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Path is the path URL to check for the pool members in the format `/healthz`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// Port is the port this monitor should check the pool members
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port"`

	// MonitorType is the monitor parent type. <monitorType> must be one of "http", "https",
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// "icmp".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=http;https;icmp
	MonitorType string `json:"monitortype"`
}

// Provider is a backend provider for F5 Big IP Load Balancers
type Provider struct {
	// Vendor is the backend provider vendor
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	Vendor string `json:"vendor"`

	// Host is the Load Balancer API IP or Hostname in URL format. Eg. `http://10.25.10.10`.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Host string `json:"host"`

	// Port is the Load Balancer API Port.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port"`

	// Creds is the credentials secret holding the "username" and "password" keys.
	// Generate with: `kubectl create secret generic <secret-name> --from-literal=username=<username> --from-literal=password=<password>`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Required
	Creds string `json:"creds"`

	// Partition is the F5 partition to create the Load Balancer instances. Defaults to "Common". (F5 BigIP only)
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	Partition string `json:"partition,omitempty"`

	// ValidateCerts is a flag to validate or not the Load Balancer API certificate. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Enum=true;false
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	ValidateCerts bool `json:"validatecerts,omitempty"`

	// Debug is a flag to enable debug on the backend log output. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=true;false
	// +kubebuilder:default=false
	Debug bool `json:"debug,omitempty"`

	// Type is the Load-Balancing method. Defaults to "round-robin".
	// Options are: ROUNDROBIN, LEASTCONNECTION, LEASTRESPONSETIME
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=ROUNDROBIN;LEASTCONNECTION;LEASTRESPONSETIME
	// +kubebuilder:default=ROUNDROBIN
	LBMethod string `json:"lbmethod,omitempty"`
}

// Internal types

// Pool defines a pool object in the LoadBalancer.
type Pool struct {
	// Name is the Pool name, it is set by the controller
	Name string `json:"name,omitempty"`
	// Members is the host members of this pool
	Members []PoolMember `json:"members,omitempty"`
	// Monitor is the monitor name used on this pool
	Monitor string `json:"monitor"`
}

// Node defines a host object in the LoadBalancer.
type Node struct {
	// Name is the host name set dynamically by the controller
	Name string `json:"name,omitempty"`
	// Host is the host IP set dynamically by the controller
	Host string `json:"host"`
	// Label is the node labels this node has
	Labels map[string]string `json:"label,omitempty"`
}

// PoolMember defines a host object in the LoadBalancer.
type PoolMember struct {
	// Node is the node part of a pool
	Node Node `json:"node"`
	// Port is the port for this pool member
	Port int `json:"port"`
}

// VIP defines VIP instance in the LoadBalancer with a pool and port
type VIP struct {
	// Name is the VIP instance name
	Name string `json:"name"`
	// Pool is the associated pool with this VIP
	Pool string `json:"pool"`
	// IP is the IP address this VIP instance listens to
	IP string `json:"ip"`
	// Port is the port this VIP listens to
	Port int `json:"port"`
}

// ExternalLoadBalancerStatus defines the observed state of ExternalLoadBalancer
type ExternalLoadBalancerStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	VIPs []VIP `json:"vips"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Ports []int `json:"ports"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Monitor Monitor `json:"monitor"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Nodes []Node `json:"nodes,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Pools []Pool `json:"pools,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Provider Provider `json:"provider,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Labels map[string]string `json:"labels,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=status
	NumNodes int `json:"numnodes,omitempty"`
}

// +kubebuilder:object:root=true

// ExternalLoadBalancerList contains a list of ExternalLoadBalancer
type ExternalLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalLoadBalancer `json:"items"`
}
