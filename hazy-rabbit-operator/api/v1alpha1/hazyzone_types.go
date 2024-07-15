/*
Copyright 2024.

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
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HazyZoneSpec defines the desired state of HazyZone
type HazyZoneSpec struct {
	VHost    string   `json:"vHost,omitempty"`
	Exchange string   `json:"exchange,omitempty"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Queues   []string `json:"queues"`
}

// HazyZoneStatus defines the observed state of HazyZone
type HazyZoneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HazyZone is the Schema for the hazyzones API
type HazyZone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HazyZoneSpec   `json:"spec,omitempty"`
	Status HazyZoneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HazyZoneList contains a list of HazyZone
type HazyZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HazyZone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HazyZone{}, &HazyZoneList{})
}

func FillDefaultsHazyZoneSPec(hz *HazyZone, ns string) *HazyZone {
	spec := hz.Spec
	if len(spec.VHost) == 0 {
		spec.VHost = ns
	}
	if len(spec.Exchange) == 0 {
		spec.Exchange = ns
	}

	if len(spec.Username) == 0 {
		spec.Username = ns
	}
	if len(spec.Password) == 0 {
		spec.Password = uuid.New().String()
	}

	hz.Spec = spec

	return hz
}
