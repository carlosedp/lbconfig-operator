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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ExternalLoadBalancer is the Schema for the externalloadbalancers API
// +kubebuilder:subresource:status
type ExternalLoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalLoadBalancerSpec   `json:"spec,omitempty"`
	Status ExternalLoadBalancerStatus `json:"status,omitempty"`
}

// ExternalLoadBalancerSpec is the spec of a LoadBalancer instance.
type ExternalLoadBalancerSpec struct {
	// Vip is the Virtual IP configured in  this LoadBalancer instance
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=7
	// +kubebuilder:validation:MaxLength=15
	Vip string `json:"vip"`

	// Type is the node role type (master or infra) for the LoadBalancer instance
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=`master`;`infra`
	Type string `json:"type,omitempty"`

	// NodeLabels are the node labels used for router sharding or exposed service. Optional.
	// +kubebuilder:validation:Optional
	NodeLabels map[string]string `json:"nodelabels,omitempty"`

	// Backend is the LoadBalancer used
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=128
	Ports []int `json:"ports"`

	// Monitor is the path and port to monitor the LoadBalancer members
	// +kubebuilder:validation:Required
	Monitor Monitor `json:"monitor"`

	// Provider is the LoadBalancer backend provider
	// +kubebuilder:validation:Required
	Provider Provider `json:"provider"`
}

// Monitor defines a monitor object in the LoadBalancer.
type Monitor struct {
	// Name is the monitor name, it is set by the controller
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Path is the path URL to check for the pool members
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Port is the port this monitor should check the pool members
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port"`

	// MonitorType is the monitor parent type. <monitorType> must be one of "http", "https",
	// "icmp".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=`http`;`https`;`icmp`
	MonitorType string `json:"monitortype"`
}

// Provider is a backend provider for F5 Big IP Load Balancers
type Provider struct {
	// Vendor is the backend provider vendor
	// +kubebuilder:validation:Required
	Vendor string `json:"vendor"`

	// Host is the Load Balancer API IP or Hostname.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Host string `json:"host"`

	// Port is the Load Balancer API Port.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port"`

	// Creds credentials secret holding the username and password keys.
	// +kubebuilder:validation:Required
	Creds string `json:"creds"`

	// Partition is the F5 partition to create the Load Balancer instances.
	// +kubebuilder:validation:Optional
	Partition string `json:"partition,omitempty"`

	// ValidateCerts is a flag to validate or not the Load Balancer API certificate. Defaults to false.
	// +kubebuilder:validation:Optional
	ValidateCerts *bool `json:"validatecerts,omitempty"`
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
	VIPs     []VIP    `json:"vips"`
	Ports    []int    `json:"ports"`
	Monitor  Monitor  `json:"monitor"`
	Nodes    []Node   `json:"nodes,omitempty"`
	Pools    []Pool   `json:"pools,omitempty"`
	Provider Provider `json:"provider,omitempty"`
}

// +kubebuilder:object:root=true

// ExternalLoadBalancerList contains a list of ExternalLoadBalancer
type ExternalLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalLoadBalancer `json:"items"`
}
