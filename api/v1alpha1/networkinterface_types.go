/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkInterfaceSpec defines the desired state of NetworkInterface
type NetworkInterfaceSpec struct {
	// ID is the ID of the NIC
	ID string `json:"id"`

	// NodeName is the name of the node the interface is attached to
	NodeName string `json:"nodeName"`

	// Address is the address of the interface
	// deprecated
	Address string `json:"address,omitempty"`
}

// NetworkInterfaceStatus defines the observed state of NetworkInterface
type NetworkInterfaceStatus struct {
	// LinkName is the name of the Interface
	LinkName string `json:"linkName"`

	// MacAddress is the mac address of the interface
	MacAddress string `json:"macAddress"`

	// Address is the address of the interface
	Address string `json:"address,omitempty"`

	// ParentCIDR is the parent cidr of the Address
	ParentCIDR string `json:"parentCidr,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ni;nif;networkinterface;netiface;niface
// +kubebuilder:printcolumn:name="address",type="string",JSONPath=".status.address"
// +kubebuilder:printcolumn:name="node name",type="string",JSONPath=".spec.nodeName"
// +kubebuilder:printcolumn:name="mac address",type="string",JSONPath=".status.macAddress"
// +kubebuilder:printcolumn:name="link name",type="string",JSONPath=".status.linkName"

// NetworkInterface is the Schema for the networkinterfaces API
type NetworkInterface struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkInterfaceSpec   `json:"spec,omitempty"`
	Status NetworkInterfaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkInterfaceList contains a list of NetworkInterface
type NetworkInterfaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkInterface `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkInterface{}, &NetworkInterfaceList{})
}
