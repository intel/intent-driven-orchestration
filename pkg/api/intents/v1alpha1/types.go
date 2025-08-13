package v1alpha1

import (
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Objectives

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Intent kind definition.
type Intent struct {
	metaV1.TypeMeta   `json:",inline"`
	metaV1.ObjectMeta `json:"metadata,omitempty"`

	Spec IntentSpec `json:"spec"`
	// TODO: add status object.
}

// IntentSpec represent the actual Intent spec.
type IntentSpec struct {
	TargetRef       TargetRef         `json:"targetRef"`
	Priority        float64           `json:"priority"`
	ActivelyManaged bool              `json:"active"`
	Objectives      []TargetObjective `json:"objectives"`
}

// TargetRef represent the data needed to find the related object.
type TargetRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// TargetObjective represent the actual objective.
type TargetObjective struct {
	Name       string  `json:"name"`
	Value      float64 `json:"value"`
	MeasuredBy string  `json:"measuredBy"`
	Tolerance  float64 `json:"tolerance"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IntentList is a list of Intent resources.
type IntentList struct {
	metaV1.TypeMeta `json:",inline"`
	metaV1.ListMeta `json:"metadata"`

	Items []Intent `json:"items"`
}

// KPIProfile

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KPIProfile for KPI kind definition.
type KPIProfile struct {
	metaV1.TypeMeta   `json:",inline"`
	metaV1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KPIProfileSpec   `json:"spec"`
	Status KPIProfileStatus `json:"status"`
}

// KPIProfileSpec represent the actual KPIProfile spec.
type KPIProfileSpec struct {
	Query       string            `json:"query"`
	Description string            `json:"description"`
	Minimize    bool              `json:"minimize"`
	KPIType     string            `json:"type"`
	Props       map[string]string `json:"props"`
}

// KPIProfileStatus represent the status object.
type KPIProfileStatus struct {
	Resolved bool   `json:"resolved"`
	Reason   string `json:"reason"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KPIProfileList is a list of KPIProfile resources.
type KPIProfileList struct {
	metaV1.TypeMeta `json:",inline"`
	metaV1.ListMeta `json:"metadata"`

	Items []KPIProfile `json:"items"`
}
