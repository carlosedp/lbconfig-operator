package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExternalLoadBalancerSpec is the spec of a LoadBalancer instance.
type ExternalLoadBalancerSpec struct {
	// Vip is the Virtual IP configured in  this LoadBalancer instance
	Vip string `json:"vip"`
	// Type is the node role type (master or infra) for the LoadBalancer instance
	Type string `json:"type"`
	// ShardLabels are the Infra node labels used for router sharding. Optional.
	ShardLabels map[string]string `json:"shardlabels,omitempty"`
	// Backend is the LoadBalancer used
	Backend string `json:"backend"`
	// Ports are the list of ports used for this Vip
	Ports []int `json:"ports"`
	// Monitor is the path and port to monitor the LoadBalancer members
	Monitor Monitor `json:"monitor"`
}

// Monitor defines a monitor object in the LoadBalancer.
type Monitor struct {
	// Name is the monitor name, it is set by the controller
	Name string `json:"name,omitempty"`
	// Path is the path URL to check for the pool members
	Path string `json:"path"`
	// Port is the port this monitor should check the pool members
	Port int `json:"port"`
	// MonitorType is the monitor parent type. <monitorType> must be one of "http", "https",
	// "icmp", "gateway icmp", "inband", "postgresql", "mysql", "udp" or "tcp".
	MonitorType string `json:"monitortype"`
}

// ExternalLoadBalancerStatus defines the observed state of ExternalLoadBalancer
type ExternalLoadBalancerStatus struct {
	Vip         string   `json:"vip"`
	Ports       []int    `json:"ports"`
	Monitor     Monitor  `json:"monitor"`
	PoolMembers []string `json:"poolmembers"`
}

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

// +kubebuilder:object:root=true

// ExternalLoadBalancerList contains a list of ExternalLoadBalancer
type ExternalLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalLoadBalancer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalLoadBalancer{}, &ExternalLoadBalancerList{})
}
