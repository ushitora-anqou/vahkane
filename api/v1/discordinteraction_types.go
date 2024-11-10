package v1

import (
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type DiscordInteractionActionInline struct {
	JobTemplate batchv1.JobTemplateSpec `json:"jobTemplate"`
}

type DiscordInteractionAction struct {
	ActionInline DiscordInteractionActionInline `json:"actionInline"`
	Pattern      string                         `json:"pattern"`
}

// DiscordInteractionSpec defines the desired state of DiscordInteraction.
type DiscordInteractionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	GuildID  string                     `json:"guildID"`
	Actions  []DiscordInteractionAction `json:"actions"`
	Commands string                     `json:"commands"`
}

// DiscordInteractionStatus defines the observed state of DiscordInteraction.
type DiscordInteractionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DiscordInteraction is the Schema for the discordinteractions API.
type DiscordInteraction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DiscordInteractionSpec   `json:"spec,omitempty"`
	Status DiscordInteractionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DiscordInteractionList contains a list of DiscordInteraction.
type DiscordInteractionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiscordInteraction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DiscordInteraction{}, &DiscordInteractionList{})
}
