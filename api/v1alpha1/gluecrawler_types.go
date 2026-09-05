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

// GlueS3Target specifies an S3 data store to crawl.
type GlueS3Target struct {
	// Path is the path to the Amazon S3 target (e.g. s3://bucket/prefix).
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// Exclusions is a list of glob patterns to exclude from the crawl.
	// +optional
	Exclusions []string `json:"exclusions,omitempty"`
}

// GlueJDBCTarget specifies a JDBC data store to crawl.
type GlueJDBCTarget struct {
	// ConnectionName is the name of the Glue connection to use.
	// +kubebuilder:validation:MinLength=1
	ConnectionName string `json:"connectionName"`

	// Path is the path of the JDBC target.
	// +optional
	Path string `json:"path,omitempty"`

	// Exclusions is a list of glob patterns to exclude from the crawl.
	// +optional
	Exclusions []string `json:"exclusions,omitempty"`
}

// GlueCrawlerTargets is the collection of data stores to crawl.
type GlueCrawlerTargets struct {
	// S3Targets specifies Amazon S3 targets.
	// +optional
	S3Targets []GlueS3Target `json:"s3Targets,omitempty"`

	// JDBCTargets specifies JDBC targets.
	// +optional
	JDBCTargets []GlueJDBCTarget `json:"jdbcTargets,omitempty"`
}

// GlueSchemaChangePolicy sets the crawler's update and deletion behavior.
type GlueSchemaChangePolicy struct {
	// UpdateBehavior is the behavior when the crawler finds a changed schema.
	// +kubebuilder:validation:Enum=LOG;UPDATE_IN_DATABASE
	// +optional
	UpdateBehavior string `json:"updateBehavior,omitempty"`

	// DeleteBehavior is the behavior when the crawler finds a deleted object.
	// +kubebuilder:validation:Enum=LOG;DELETE_FROM_DATABASE;DEPRECATE_IN_DATABASE
	// +optional
	DeleteBehavior string `json:"deleteBehavior,omitempty"`
}

// GlueCrawlerSpec defines the desired state of a Glue crawler.
type GlueCrawlerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the crawler. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// RoleRef references the IAM role the crawler uses to access resources
	// (either a managed IAMRole CR or a direct ARN).
	RoleRef RoleRef `json:"roleRef"`

	// DatabaseName is the Glue database where crawl results are written.
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

	// DatabaseRef is the name of a GlueDatabase CR in the same namespace.
	// Ignored when DatabaseName is set.
	// +optional
	DatabaseRef string `json:"databaseRef,omitempty"`

	// Targets is the collection of data stores to crawl.
	Targets GlueCrawlerTargets `json:"targets"`

	// Schedule is a cron expression, e.g. cron(15 12 * * ? *).
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// SchemaChangePolicy sets the crawler's update and deletion behavior.
	// +optional
	SchemaChangePolicy *GlueSchemaChangePolicy `json:"schemaChangePolicy,omitempty"`

	// TablePrefix is the prefix for catalog tables the crawler creates.
	// +optional
	TablePrefix string `json:"tablePrefix,omitempty"`

	// Configuration is a versioned JSON configuration string for the crawler.
	// +optional
	Configuration string `json:"configuration,omitempty"`

	// Description is a description of the crawler.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GlueCrawlerStatus defines the observed state of GlueCrawler.
type GlueCrawlerStatus struct {
	// CrawlerName is the name of the crawler in AWS.
	// +optional
	CrawlerName string `json:"crawlerName,omitempty"`

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
// +kubebuilder:printcolumn:name="Crawler",type="string",JSONPath=".status.crawlerName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GlueCrawler is the Schema for managing Glue crawlers.
type GlueCrawler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlueCrawlerSpec   `json:"spec,omitempty"`
	Status GlueCrawlerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlueCrawlerList contains a list of GlueCrawler
type GlueCrawlerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlueCrawler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlueCrawler{}, &GlueCrawlerList{})
}
