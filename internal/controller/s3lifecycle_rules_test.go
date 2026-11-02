package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func TestBuildLifecycleRule(t *testing.T) {
	t.Run("no prefix still sends a Filter with an empty prefix", func(t *testing.T) {
		lr, err := buildLifecycleRule(awsv1alpha1.S3LifecycleRule{
			ID: "expire-all", Status: "Enabled",
			ExpirationDays:                     aws.Int32(90),
			NoncurrentVersionExpirationDays:    aws.Int32(1),
			AbortIncompleteMultipartUploadDays: aws.Int32(7),
		})
		if err != nil {
			t.Fatal(err)
		}
		if lr.Filter == nil || lr.Filter.Prefix == nil || *lr.Filter.Prefix != "" {
			t.Fatalf("expected Filter{Prefix:\"\"}, got %+v", lr.Filter)
		}
		if aws.ToString(lr.ID) != "expire-all" || lr.Status != s3types.ExpirationStatusEnabled {
			t.Errorf("id/status not mapped: %+v", lr)
		}
		if lr.Expiration == nil || aws.ToInt32(lr.Expiration.Days) != 90 || lr.Expiration.Date != nil {
			t.Errorf("expirationDays not mapped: %+v", lr.Expiration)
		}
		if lr.NoncurrentVersionExpiration == nil || aws.ToInt32(lr.NoncurrentVersionExpiration.NoncurrentDays) != 1 {
			t.Errorf("noncurrent expiration not mapped: %+v", lr.NoncurrentVersionExpiration)
		}
		if lr.AbortIncompleteMultipartUpload == nil || aws.ToInt32(lr.AbortIncompleteMultipartUpload.DaysAfterInitiation) != 7 {
			t.Errorf("abort multipart not mapped: %+v", lr.AbortIncompleteMultipartUpload)
		}
	})

	t.Run("prefix is passed through", func(t *testing.T) {
		lr, err := buildLifecycleRule(awsv1alpha1.S3LifecycleRule{Status: "Enabled", Prefix: "exports/"})
		if err != nil {
			t.Fatal(err)
		}
		if aws.ToString(lr.Filter.Prefix) != "exports/" {
			t.Errorf("prefix = %q", aws.ToString(lr.Filter.Prefix))
		}
	})

	t.Run("expirationDate as plain date and RFC 3339", func(t *testing.T) {
		want := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		for _, in := range []string{"2026-12-31", "2026-12-31T15:04:05Z", "2026-12-31T23:30:00-05:00"} {
			lr, err := buildLifecycleRule(awsv1alpha1.S3LifecycleRule{Status: "Enabled", ExpirationDate: in})
			if err != nil {
				t.Fatalf("%s: %v", in, err)
			}
			if lr.Expiration == nil || lr.Expiration.Date == nil || !lr.Expiration.Date.Equal(want) {
				t.Errorf("%s: Expiration.Date = %v, want %v", in, lr.Expiration, want)
			}
			if lr.Expiration.Days != nil {
				t.Errorf("%s: Days must be unset with a date", in)
			}
		}
	})

	t.Run("expirationDays wins over expirationDate", func(t *testing.T) {
		lr, err := buildLifecycleRule(awsv1alpha1.S3LifecycleRule{Status: "Enabled", ExpirationDays: aws.Int32(3), ExpirationDate: "2026-12-31"})
		if err != nil {
			t.Fatal(err)
		}
		if lr.Expiration.Date != nil || aws.ToInt32(lr.Expiration.Days) != 3 {
			t.Errorf("got %+v", lr.Expiration)
		}
	})

	t.Run("invalid expirationDate is a clear error", func(t *testing.T) {
		_, err := buildLifecycleRule(awsv1alpha1.S3LifecycleRule{ID: "bad", Status: "Enabled", ExpirationDate: "31/12/2026"})
		if err == nil || !strings.Contains(err.Error(), `rule "bad"`) || !strings.Contains(err.Error(), "expirationDate") {
			t.Fatalf("expected descriptive error, got %v", err)
		}
	})

	t.Run("transitions and noncurrent version transitions", func(t *testing.T) {
		lr, err := buildLifecycleRule(awsv1alpha1.S3LifecycleRule{
			Status: "Enabled",
			Transitions: []awsv1alpha1.S3LifecycleTransition{
				{Days: aws.Int32(30), StorageClass: "STANDARD_IA"},
				{Date: "2027-01-01", StorageClass: "GLACIER"},
			},
			NoncurrentVersionTransitions: []awsv1alpha1.S3NoncurrentVersionTransition{
				{NoncurrentDays: aws.Int32(14), StorageClass: "GLACIER_IR"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(lr.Transitions) != 2 || aws.ToInt32(lr.Transitions[0].Days) != 30 || lr.Transitions[0].StorageClass != s3types.TransitionStorageClassStandardIa {
			t.Errorf("transitions[0] = %+v", lr.Transitions)
		}
		if lr.Transitions[1].Date == nil || lr.Transitions[1].Date.Year() != 2027 || lr.Transitions[1].StorageClass != s3types.TransitionStorageClassGlacier {
			t.Errorf("transitions[1] = %+v", lr.Transitions[1])
		}
		if len(lr.NoncurrentVersionTransitions) != 1 ||
			aws.ToInt32(lr.NoncurrentVersionTransitions[0].NoncurrentDays) != 14 ||
			lr.NoncurrentVersionTransitions[0].StorageClass != s3types.TransitionStorageClassGlacierIr {
			t.Errorf("noncurrent transitions = %+v", lr.NoncurrentVersionTransitions)
		}
	})

	t.Run("buildLifecycleRules fails on the first invalid rule", func(t *testing.T) {
		_, err := buildLifecycleRules([]awsv1alpha1.S3LifecycleRule{
			{ID: "ok", Status: "Enabled", ExpirationDays: aws.Int32(1)},
			{ID: "bad", Status: "Enabled", ExpirationDate: "never"},
		})
		if err == nil || !strings.Contains(err.Error(), `rule "bad"`) {
			t.Fatalf("got %v", err)
		}
	})
}
