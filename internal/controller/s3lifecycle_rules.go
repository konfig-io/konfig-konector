/*
Copyright 2025.

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

package controller

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// parseLifecycleDate accepts a plain date (2026-12-31) or an RFC 3339
// timestamp and returns midnight UTC of that day, which is what S3 requires.
func parseLifecycleDate(field, value string) (*time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			return &d, nil
		}
	}
	return nil, fmt.Errorf("%s %q: expected an ISO 8601 date (2026-12-31) or RFC 3339 timestamp", field, value)
}

// buildLifecycleRule maps one S3LifecycleRule from the API into the SDK
// shape. Every rule carries a Filter: S3 rejects the whole configuration with
// MalformedXML when a rule has neither Filter nor the legacy Prefix, and an
// empty prefix means "the whole bucket".
func buildLifecycleRule(rule awsv1alpha1.S3LifecycleRule) (s3types.LifecycleRule, error) {
	lr := s3types.LifecycleRule{
		Status: s3types.ExpirationStatus(rule.Status),
		Filter: &s3types.LifecycleRuleFilter{Prefix: aws.String(rule.Prefix)},
	}
	if rule.ID != "" {
		lr.ID = aws.String(rule.ID)
	}
	switch {
	case rule.ExpirationDays != nil:
		lr.Expiration = &s3types.LifecycleExpiration{Days: rule.ExpirationDays}
	case rule.ExpirationDate != "":
		d, err := parseLifecycleDate("expirationDate", rule.ExpirationDate)
		if err != nil {
			return s3types.LifecycleRule{}, fmt.Errorf("rule %q: %w", rule.ID, err)
		}
		lr.Expiration = &s3types.LifecycleExpiration{Date: d}
	}
	if rule.NoncurrentVersionExpirationDays != nil {
		lr.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{
			NoncurrentDays: rule.NoncurrentVersionExpirationDays,
		}
	}
	if rule.AbortIncompleteMultipartUploadDays != nil {
		lr.AbortIncompleteMultipartUpload = &s3types.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: rule.AbortIncompleteMultipartUploadDays,
		}
	}
	for _, t := range rule.Transitions {
		tr := s3types.Transition{StorageClass: s3types.TransitionStorageClass(t.StorageClass)}
		switch {
		case t.Days != nil:
			tr.Days = t.Days
		case t.Date != "":
			d, err := parseLifecycleDate("transition date", t.Date)
			if err != nil {
				return s3types.LifecycleRule{}, fmt.Errorf("rule %q: %w", rule.ID, err)
			}
			tr.Date = d
		}
		lr.Transitions = append(lr.Transitions, tr)
	}
	for _, nvt := range rule.NoncurrentVersionTransitions {
		lr.NoncurrentVersionTransitions = append(lr.NoncurrentVersionTransitions, s3types.NoncurrentVersionTransition{
			StorageClass:   s3types.TransitionStorageClass(nvt.StorageClass),
			NoncurrentDays: nvt.NoncurrentDays,
		})
	}
	return lr, nil
}

// buildLifecycleRules maps a rule list, failing on the first invalid rule.
func buildLifecycleRules(rules []awsv1alpha1.S3LifecycleRule) ([]s3types.LifecycleRule, error) {
	out := make([]s3types.LifecycleRule, 0, len(rules))
	for _, r := range rules {
		lr, err := buildLifecycleRule(r)
		if err != nil {
			return nil, err
		}
		out = append(out, lr)
	}
	return out, nil
}
