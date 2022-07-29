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

// LoadBalancerBackendSpec defines the backend used by the ExternalLoadBalancer instance
type LoadBalancerBackendSpec struct {
	// Type is the backend provider like F5, NetScaler, NSX
	Provider Provider `json:"provider"`
}

// Provider is the interface to different backend providers (F5, NetScaler, NSX, etc)
// type Provider struct {
// }

// Provider is a backend provider for F5 Big IP Load Balancers
type Provider struct {
	// Vendor is the backend provider vendor (F5, NSX, Netscaler)
	Vendor string `json:"vendor"`
	// Host is the Load Balancer API IP or Hostname.
	Host string `json:"host"`
	// Port is the Load Balancer API Port.
	Port int `json:"port"`
	// Creds credentials secret holding the username and password keys.
	Creds string `json:"creds"`
	// Partition is the F5 partition to create the Load Balancer instances.
	Partition string `json:"partition,omitempty"`
	// ValidateCerts is a flag to validate or not the Load Balancer API certificate. Defaults to false.
	// +optional
	ValidateCerts *bool `json:"validatecerts,omitempty"`
}

// LoadBalancerBackendStatus defines the observed state of LoadBalancerBackend
type LoadBalancerBackendStatus struct {
	Type Provider `json:"provider"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// LoadBalancerBackend is the Schema for the loadbalancerbackends API
// +kubebuilder:subresource:status
type LoadBalancerBackend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerBackendSpec   `json:"spec,omitempty"`
	Status LoadBalancerBackendStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoadBalancerBackendList contains a list of LoadBalancerBackend
type LoadBalancerBackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancerBackend `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoadBalancerBackend{}, &LoadBalancerBackendList{})
}
