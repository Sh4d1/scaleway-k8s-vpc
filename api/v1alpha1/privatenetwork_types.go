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

// PrivateNetworkSpec defines the desired state of PrivateNetwork
type PrivateNetworkSpec struct {
	// ID is the ID of the PrivateNetwork
	ID string `json:"id"`

	// Zone is the Zone of the PrivateNetwork
	// Will default to the SCW_DEFAULT_ZONE env variable
	// +optional
	Zone string `json:"zone,omitempty"`

	IPAM *PrivateNetworkIPAM `json:"ipam,omitempty"`

	// Routes are the routes injected in the cluster to this PrivateNetwork
	// +optional
	Routes []PrivateNetworkRoute `json:"routes,omitempty"`

	// Masquerade represents whether the private network needs to be masqueraded
	// +optional
	// +kubebuilder:default:=true
	Masquerade bool `json:"masquerade,omitempty"`

	// CIDR is the CIDR of the PrivateNetwork
	// deprecated
	CIDR string `json:"cidr,omitempty"`
}

// PrivateNetworkRoute defines a route from the PrivateNetwork
type PrivateNetworkRoute struct {
	To  string `json:"to"`
	Via string `json:"via"`
}

// +kubebuilder:validation:Enum=DHCP;Static
// IPAMType represents a type of IPAM
type IPAMType string

const (
	// IPAMTypeDHCP represents the dhcp IPAM type
	IPAMTypeDHCP IPAMType = "DHCP"
	// IPAMTypeStatic represents the static IPAM type
	IPAMTypeStatic IPAMType = "Static"
)

type PrivateNetworkIPAMStatic struct {
	// CIDR represents the CIDR associated to this private network
	CIDR string `json:"cidr"`
	// AvailableRanges allows to restrict which ranges of addresses should be used when choosing an IP address
	// Defaults to the whole CIDR
	AvailableRanges []string `json:"availableRanges,omitempty"`
}

// PrivateNetworkIPAM defines the IPAM for the PrivateNetwork
type PrivateNetworkIPAM struct {
	Type   IPAMType                  `json:"type"`
	Static *PrivateNetworkIPAMStatic `json:"static,omitempty"`
}

// PrivateNetworkStatus defines the observed state of PrivateNetwork
type PrivateNetworkStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pn;privnet;privatenet;privatenetwork
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".spec.id"
// +kubebuilder:printcolumn:name="ipam type",type="string",JSONPath=".spec.ipam.type"

// PrivateNetwork is the Schema for the privatenetworks API
type PrivateNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateNetworkSpec   `json:"spec,omitempty"`
	Status PrivateNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PrivateNetworkList contains a list of PrivateNetwork
type PrivateNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivateNetwork `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrivateNetwork{}, &PrivateNetworkList{})
}
