package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InfraLoadBalancerSpec is the spec of a LoadBalancer instance.
type InfraLoadBalancerSpec struct {
	// Vip is the Virtual IP configured in  this LoadBalancer instance
	Vip string `json:"vip"`
	// Type is the node role type (master or infra) for the LoadBalancer instance
	Type LoadBalancerType `json:"type"`
	// Backend is the LoadBalancer used
	Backend string `json:"backend"`
	// Ports are the list of ports used for this Vip
	Ports []int32 `json:"ports"`
	// Monitor is the path and port to monitor the LoadBalancer members
	Monitor Monitor `json:"monitor"`
}

// LoadBalancerType is the type of the Load Balancer based on node role
type LoadBalancerType string

const (
	// MasterLB is the Load Balancer that is created for Master nodes
	MasterLB LoadBalancerType = "master"
	// InfraLB is the Load Balancer that is created for Infra nodes
	InfraLB LoadBalancerType = "infra"
)

// Monitor defines a monitor object in the LoadBalancer.
type Monitor struct {
	// Path is the path URL to check for the pool members
	Path string `json:"path"`
	// Port is the port this monitor should check the pool members
	Port int32 `json:"port"`
}

// InfraLoadBalancerStatus defines the observed state of InfraLoadBalancer
type InfraLoadBalancerStatus struct {
	Vip         string   `json:"vip"`
	Monitor     string   `json:"monitor"`
	PoolMembers []string `json:"poolmembers"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// InfraLoadBalancer is the Schema for the infraloadbalancers API
// +kubebuilder:subresource:status
type InfraLoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InfraLoadBalancerSpec   `json:"spec,omitempty"`
	Status InfraLoadBalancerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InfraLoadBalancerList contains a list of InfraLoadBalancer
type InfraLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InfraLoadBalancer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InfraLoadBalancer{}, &InfraLoadBalancerList{})
}
