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

package controllers

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// -----------------------------------------
// Auxiliary functions
// -----------------------------------------

// contains check if string s is in array list
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// hasNodeChanged checks two instances of node and compares if some fields have changed
func hasNodeChanged(o *corev1.Node, n *corev1.Node) bool {
	var oldCond corev1.ConditionStatus
	var newCond corev1.ConditionStatus
	var oldIP string
	var newIP string

	for _, cond := range o.Status.Conditions {
		if cond.Type == readyCondition {
			oldCond = cond.Status
		}
	}
	for _, cond := range n.Status.Conditions {
		if cond.Type == readyCondition {
			newCond = cond.Status
		}
	}
	oldIP = getNodeIP(o)
	newIP = getNodeIP(n)

	if (oldCond == newCond) && (oldIP == newIP) && reflect.DeepEqual(o.Labels, n.Labels) {
		return false
	}
	return true
}

func getNodeIP(node *corev1.Node) string {
	var nodeReady = false
	var nodeIPs = make(map[corev1.NodeAddressType]string)
	var IP string
	for _, cond := range node.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			nodeReady = true
		}
	}
	for _, addr := range node.Status.Addresses {
		if nodeReady {
			nodeIPs[addr.Type] = addr.Address
		}
	}
	if val, ok := nodeIPs["ExternalIP"]; ok {
		IP = val
	} else {
		IP = nodeIPs["InternalIP"]
	}
	return IP
}

// computeLabels builds a label map with node role and additional labels
func computeLabels(lb lbv1.ExternalLoadBalancer) map[string]string {
	labels := make(map[string]string)
	if lb.Spec.Type != "" {
		labels["node-role.kubernetes.io/"+lb.Spec.Type] = ""
	}
	if lb.Spec.NodeLabels != nil {
		for k, v := range lb.Spec.NodeLabels {
			labels[k] = v
		}
	}
	return labels
}

// containsLabels checks if label map `as` contains labels from map `bs`
func containsLabels(as, bs map[string]string) bool {
	labels := make(map[string]string)
	for k, v := range bs {
		if _, ok := as[k]; ok {
			labels[k] = v
		}
	}
	return reflect.DeepEqual(bs, labels)
}
