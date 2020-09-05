package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoadBalancerBackendSpec defines the backend used by the ExternalLoadBalancer instance
type LoadBalancerBackendSpec struct {
	// Type is the backend provider like F5, NetScaler, NSX
	Provider F5Provider `json:"provider"`
}

// Provider is the interface to different backend providers (F5, NetScaler, NSX, etc)
// type Provider struct {
// }

// F5Provider is a backend provider for F5 Big IP Load Balancers
type F5Provider struct {
	// Vendor is the backend provider vendor (F5, NSX, Nerscaler)
	Vendor string `json:"vendor"`
	// Host is the Load Balancer API IP or Hostname.
	Host string `json:"host"`
	// Port is the Load Balancer API Port.
	Port int `json:"hostport"`
	// Partition is the F5 partition to create the Load Balancer instances.
	Partition string `json:"partition"`
	// ValidateCerts is a flag to validate or not the Load Balancer API certificate. Defaults to false.
	// +optional
	ValidateCerts *bool `json:"validatecerts,omitempty"`
}

// LoadBalancerBackendStatus defines the observed state of LoadBalancerBackend
type LoadBalancerBackendStatus struct {
	Type F5Provider `json:"provider"`
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
