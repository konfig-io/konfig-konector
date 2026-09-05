/*
Copyright 2026.

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

// GlueJobCommand specifies the code the Glue job runs.
type GlueJobCommand struct {
	// Name is the job command name: glueetl for Spark ETL, pythonshell for
	// Python shell, gluestreaming for streaming ETL.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// ScriptLocation is the S3 path of the script that runs the job.
	// +kubebuilder:validation:MinLength=1
	ScriptLocation string `json:"scriptLocation"`

	// PythonVersion is the Python version for pythonshell jobs (2 or 3).
	// +optional
	PythonVersion string `json:"pythonVersion,omitempty"`
}

// GlueJobSpec defines the desired state of a Glue job.
type GlueJobSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the job. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// RoleRef references the IAM role the job uses (either a managed IAMRole
	// CR or a direct ARN).
	RoleRef RoleRef `json:"roleRef"`

	// Command specifies the code the job runs.
	Command GlueJobCommand `json:"command"`

	// DefaultArguments are default arguments for job runs.
	// +optional
	DefaultArguments map[string]string `json:"defaultArguments,omitempty"`

	// MaxRetries is the maximum number of retries for a failing job run.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// Timeout is the job run timeout in minutes.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`

	// GlueVersion determines the Glue runtime versions (e.g. "4.0").
	// +optional
	GlueVersion string `json:"glueVersion,omitempty"`

	// NumberOfWorkers is the number of workers allocated when the job runs.
	// +kubebuilder:validation:Minimum=1
	// +optional
	NumberOfWorkers *int32 `json:"numberOfWorkers,omitempty"`

	// WorkerType is the type of predefined worker allocated when the job runs.
	// +kubebuilder:validation:Enum=Standard;G.1X;G.2X;G.025X;G.4X;G.8X;Z.2X
	// +optional
	WorkerType string `json:"workerType,omitempty"`

	// Description is a description of the job.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GlueJobStatus defines the observed state of GlueJob.
type GlueJobStatus struct {
	// JobName is the name of the job in AWS.
	// +optional
	JobName string `json:"jobName,omitempty"`

	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is when the resource was last successfully reconciled.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Job",type="string",JSONPath=".status.jobName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GlueJob is the Schema for managing Glue jobs.
type GlueJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlueJobSpec   `json:"spec,omitempty"`
	Status GlueJobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlueJobList contains a list of GlueJob
type GlueJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlueJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlueJob{}, &GlueJobList{})
}
