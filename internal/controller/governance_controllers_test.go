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

package controller

// Create + delete + abandon coverage for the governance family kinds that do
// not have full test suites (Trail and BackupPlan carry the full suites).

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsbackup "github.com/aws/aws-sdk-go-v2/service/backup"
	awsconfigservice "github.com/aws/aws-sdk-go-v2/service/configservice"
	configtypes "github.com/aws/aws-sdk-go-v2/service/configservice/types"
	awsguardduty "github.com/aws/aws-sdk-go-v2/service/guardduty"
	awsinspector2 "github.com/aws/aws-sdk-go-v2/service/inspector2"
	inspector2types "github.com/aws/aws-sdk-go-v2/service/inspector2/types"
	awssecurityhub "github.com/aws/aws-sdk-go-v2/service/securityhub"
	securityhubtypes "github.com/aws/aws-sdk-go-v2/service/securityhub/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func govFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := newGovScheme(t)
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&awsv1alpha1.ConfigRecorder{},
			&awsv1alpha1.ConfigDeliveryChannel{},
			&awsv1alpha1.ConfigRule{},
			&awsv1alpha1.BackupVault{},
			&awsv1alpha1.BackupPlan{},
			&awsv1alpha1.BackupSelection{},
			&awsv1alpha1.GuardDutyDetector{},
			&awsv1alpha1.SecurityHubAccount{},
			&awsv1alpha1.SecurityHubStandard{},
			&awsv1alpha1.InspectorEnabler{},
		).
		WithObjects(objs...).
		Build()
}

func assertReadyTrue(t *testing.T, conds []metav1.Condition) {
	t.Helper()
	cond := apimeta.FindStatusCondition(conds, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func assertGone(t *testing.T, ctx context.Context, c client.Client, key k8stypes.NamespacedName, obj client.Object) {
	t.Helper()
	if err := c.Get(ctx, key, obj); !apierrors.IsNotFound(err) {
		t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
	}
}

// ---------------------------------------------------------------- ConfigRecorder

type fakeConfigRecorderAPI struct {
	putCalled    bool
	deleteCalled bool
	startCalled  bool
	stopCalled   bool
	recording    bool
	deleteErr    error
}

func (f *fakeConfigRecorderAPI) DescribeConfigurationRecorders(_ context.Context, _ *awsconfigservice.DescribeConfigurationRecordersInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeConfigurationRecordersOutput, error) {
	return &awsconfigservice.DescribeConfigurationRecordersOutput{}, nil
}

func (f *fakeConfigRecorderAPI) DescribeConfigurationRecorderStatus(_ context.Context, _ *awsconfigservice.DescribeConfigurationRecorderStatusInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeConfigurationRecorderStatusOutput, error) {
	return &awsconfigservice.DescribeConfigurationRecorderStatusOutput{
		ConfigurationRecordersStatus: []configtypes.ConfigurationRecorderStatus{
			{Name: aws.String("default"), Recording: f.recording},
		},
	}, nil
}

func (f *fakeConfigRecorderAPI) PutConfigurationRecorder(_ context.Context, _ *awsconfigservice.PutConfigurationRecorderInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.PutConfigurationRecorderOutput, error) {
	f.putCalled = true
	return &awsconfigservice.PutConfigurationRecorderOutput{}, nil
}

func (f *fakeConfigRecorderAPI) DeleteConfigurationRecorder(_ context.Context, _ *awsconfigservice.DeleteConfigurationRecorderInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DeleteConfigurationRecorderOutput, error) {
	f.deleteCalled = true
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &awsconfigservice.DeleteConfigurationRecorderOutput{}, nil
}

func (f *fakeConfigRecorderAPI) StartConfigurationRecorder(_ context.Context, _ *awsconfigservice.StartConfigurationRecorderInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.StartConfigurationRecorderOutput, error) {
	f.startCalled = true
	return &awsconfigservice.StartConfigurationRecorderOutput{}, nil
}

func (f *fakeConfigRecorderAPI) StopConfigurationRecorder(_ context.Context, _ *awsconfigservice.StopConfigurationRecorderInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.StopConfigurationRecorderOutput, error) {
	f.stopCalled = true
	return &awsconfigservice.StopConfigurationRecorderOutput{}, nil
}

func configRecorderCR(mutate ...func(*awsv1alpha1.ConfigRecorder)) *awsv1alpha1.ConfigRecorder {
	cr := &awsv1alpha1.ConfigRecorder{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		Spec: awsv1alpha1.ConfigRecorderSpec{
			RecorderName: "default",
			RoleARN:      "arn:aws:iam::123456789012:role/config-role",
			RecordingGroup: &awsv1alpha1.ConfigRecordingGroup{
				AllSupported: true,
			},
		},
	}
	for _, m := range mutate {
		m(cr)
	}
	return cr
}

func TestConfigRecorderReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "default", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create puts recorder and starts recording", func(t *testing.T) {
		f := &fakeConfigRecorderAPI{}
		c := govFakeClient(t, configRecorderCR())
		r := &ConfigRecorderReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.putCalled {
			t.Error("expected PutConfigurationRecorder")
		}
		if !f.startCalled {
			t.Error("expected StartConfigurationRecorder (enabled defaults true)")
		}
		got := &awsv1alpha1.ConfigRecorder{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.RecorderName != "default" || !got.Status.Recording {
			t.Errorf("status = %+v, want recorderName default and recording", got.Status)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete removes recorder", func(t *testing.T) {
		f := &fakeConfigRecorderAPI{}
		c := govFakeClient(t, configRecorderCR(func(cr *awsv1alpha1.ConfigRecorder) {
			cr.Finalizers = []string{awsv1alpha1.FinalizerName}
			cr.Status.RecorderName = "default"
		}))
		if err := c.Delete(ctx, configRecorderCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &ConfigRecorderReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteConfigurationRecorder")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.ConfigRecorder{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		f := &fakeConfigRecorderAPI{}
		c := govFakeClient(t, configRecorderCR(func(cr *awsv1alpha1.ConfigRecorder) {
			cr.Finalizers = []string{awsv1alpha1.FinalizerName}
			cr.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		if err := c.Delete(ctx, configRecorderCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &ConfigRecorderReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteConfigurationRecorder must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.ConfigRecorder{})
	})
}

// ---------------------------------------------------------- ConfigDeliveryChannel

type fakeConfigChannelAPI struct {
	putCalled    bool
	deleteCalled bool
}

func (f *fakeConfigChannelAPI) DescribeDeliveryChannels(_ context.Context, _ *awsconfigservice.DescribeDeliveryChannelsInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeDeliveryChannelsOutput, error) {
	return &awsconfigservice.DescribeDeliveryChannelsOutput{}, nil
}

func (f *fakeConfigChannelAPI) PutDeliveryChannel(_ context.Context, params *awsconfigservice.PutDeliveryChannelInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.PutDeliveryChannelOutput, error) {
	f.putCalled = true
	if aws.ToString(params.DeliveryChannel.S3BucketName) == "" {
		return nil, fmt.Errorf("missing s3 bucket")
	}
	return &awsconfigservice.PutDeliveryChannelOutput{}, nil
}

func (f *fakeConfigChannelAPI) DeleteDeliveryChannel(_ context.Context, _ *awsconfigservice.DeleteDeliveryChannelInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DeleteDeliveryChannelOutput, error) {
	f.deleteCalled = true
	return &awsconfigservice.DeleteDeliveryChannelOutput{}, nil
}

func configChannelCR(mutate ...func(*awsv1alpha1.ConfigDeliveryChannel)) *awsv1alpha1.ConfigDeliveryChannel {
	ch := &awsv1alpha1.ConfigDeliveryChannel{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		Spec: awsv1alpha1.ConfigDeliveryChannelSpec{
			ChannelName:  "default",
			S3BucketName: "config-bucket",
		},
	}
	for _, m := range mutate {
		m(ch)
	}
	return ch
}

func TestConfigDeliveryChannelReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "default", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create puts channel", func(t *testing.T) {
		f := &fakeConfigChannelAPI{}
		c := govFakeClient(t, configChannelCR())
		r := &ConfigDeliveryChannelReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.putCalled {
			t.Error("expected PutDeliveryChannel")
		}
		got := &awsv1alpha1.ConfigDeliveryChannel{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ChannelName != "default" {
			t.Errorf("status.channelName = %q, want default", got.Status.ChannelName)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete removes channel", func(t *testing.T) {
		f := &fakeConfigChannelAPI{}
		c := govFakeClient(t, configChannelCR(func(ch *awsv1alpha1.ConfigDeliveryChannel) {
			ch.Finalizers = []string{awsv1alpha1.FinalizerName}
		}))
		if err := c.Delete(ctx, configChannelCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &ConfigDeliveryChannelReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteDeliveryChannel")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.ConfigDeliveryChannel{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		f := &fakeConfigChannelAPI{}
		c := govFakeClient(t, configChannelCR(func(ch *awsv1alpha1.ConfigDeliveryChannel) {
			ch.Finalizers = []string{awsv1alpha1.FinalizerName}
			ch.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		if err := c.Delete(ctx, configChannelCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &ConfigDeliveryChannelReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteDeliveryChannel must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.ConfigDeliveryChannel{})
	})
}

// ------------------------------------------------------------------- ConfigRule

type fakeConfigRuleAPI struct {
	putCalled    bool
	deleteCalled bool
}

func (f *fakeConfigRuleAPI) DescribeConfigRules(_ context.Context, _ *awsconfigservice.DescribeConfigRulesInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeConfigRulesOutput, error) {
	return &awsconfigservice.DescribeConfigRulesOutput{
		ConfigRules: []configtypes.ConfigRule{
			{
				ConfigRuleName: aws.String("my-rule"),
				ConfigRuleArn:  aws.String("arn:aws:config:us-east-1:123456789012:config-rule/config-rule-abc"),
				ConfigRuleId:   aws.String("config-rule-abc"),
			},
		},
	}, nil
}

func (f *fakeConfigRuleAPI) PutConfigRule(_ context.Context, params *awsconfigservice.PutConfigRuleInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.PutConfigRuleOutput, error) {
	f.putCalled = true
	if params.ConfigRule.Source == nil || params.ConfigRule.Source.Owner == "" {
		return nil, fmt.Errorf("missing source owner")
	}
	return &awsconfigservice.PutConfigRuleOutput{}, nil
}

func (f *fakeConfigRuleAPI) DeleteConfigRule(_ context.Context, _ *awsconfigservice.DeleteConfigRuleInput, _ ...func(*awsconfigservice.Options)) (*awsconfigservice.DeleteConfigRuleOutput, error) {
	f.deleteCalled = true
	return &awsconfigservice.DeleteConfigRuleOutput{}, nil
}

func configRuleCR(mutate ...func(*awsv1alpha1.ConfigRule)) *awsv1alpha1.ConfigRule {
	rule := &awsv1alpha1.ConfigRule{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rule", Namespace: "default"},
		Spec: awsv1alpha1.ConfigRuleSpec{
			RuleName:         "my-rule",
			SourceOwner:      "AWS",
			SourceIdentifier: "IAM_PASSWORD_POLICY",
		},
	}
	for _, m := range mutate {
		m(rule)
	}
	return rule
}

func TestConfigRuleReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-rule", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create puts rule and stores ARN", func(t *testing.T) {
		f := &fakeConfigRuleAPI{}
		c := govFakeClient(t, configRuleCR())
		r := &ConfigRuleReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.putCalled {
			t.Error("expected PutConfigRule")
		}
		got := &awsv1alpha1.ConfigRule{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.RuleARN == "" || got.Status.RuleID == "" {
			t.Errorf("status ARN/ID not populated: %+v", got.Status)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete removes rule", func(t *testing.T) {
		f := &fakeConfigRuleAPI{}
		c := govFakeClient(t, configRuleCR(func(r *awsv1alpha1.ConfigRule) {
			r.Finalizers = []string{awsv1alpha1.FinalizerName}
		}))
		if err := c.Delete(ctx, configRuleCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &ConfigRuleReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteConfigRule")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.ConfigRule{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		f := &fakeConfigRuleAPI{}
		c := govFakeClient(t, configRuleCR(func(r *awsv1alpha1.ConfigRule) {
			r.Finalizers = []string{awsv1alpha1.FinalizerName}
			r.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		if err := c.Delete(ctx, configRuleCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &ConfigRuleReconciler{Client: c, Scheme: newGovScheme(t), ConfigClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteConfigRule must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.ConfigRule{})
	})
}

// ------------------------------------------------------------------ BackupVault

type fakeBackupVaultAPI struct {
	exists       bool
	createCalled bool
	deleteCalled bool
	tagCalled    bool
}

func (f *fakeBackupVaultAPI) DescribeBackupVault(_ context.Context, _ *awsbackup.DescribeBackupVaultInput, _ ...func(*awsbackup.Options)) (*awsbackup.DescribeBackupVaultOutput, error) {
	if !f.exists {
		return nil, &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "no vault"}
	}
	return &awsbackup.DescribeBackupVaultOutput{
		BackupVaultName: aws.String("my-vault"),
		BackupVaultArn:  aws.String("arn:aws:backup:us-east-1:123456789012:backup-vault:my-vault"),
	}, nil
}

func (f *fakeBackupVaultAPI) CreateBackupVault(_ context.Context, _ *awsbackup.CreateBackupVaultInput, _ ...func(*awsbackup.Options)) (*awsbackup.CreateBackupVaultOutput, error) {
	f.createCalled = true
	return &awsbackup.CreateBackupVaultOutput{
		BackupVaultName: aws.String("my-vault"),
		BackupVaultArn:  aws.String("arn:aws:backup:us-east-1:123456789012:backup-vault:my-vault"),
	}, nil
}

func (f *fakeBackupVaultAPI) DeleteBackupVault(_ context.Context, _ *awsbackup.DeleteBackupVaultInput, _ ...func(*awsbackup.Options)) (*awsbackup.DeleteBackupVaultOutput, error) {
	f.deleteCalled = true
	return &awsbackup.DeleteBackupVaultOutput{}, nil
}

func (f *fakeBackupVaultAPI) TagResource(_ context.Context, _ *awsbackup.TagResourceInput, _ ...func(*awsbackup.Options)) (*awsbackup.TagResourceOutput, error) {
	f.tagCalled = true
	return &awsbackup.TagResourceOutput{}, nil
}

func backupVaultCR(mutate ...func(*awsv1alpha1.BackupVault)) *awsv1alpha1.BackupVault {
	v := &awsv1alpha1.BackupVault{
		ObjectMeta: metav1.ObjectMeta{Name: "my-vault", Namespace: "default"},
		Spec:       awsv1alpha1.BackupVaultSpec{VaultName: "my-vault"},
	}
	for _, m := range mutate {
		m(v)
	}
	return v
}

func TestBackupVaultReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-vault", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create persists ARN and Ready", func(t *testing.T) {
		f := &fakeBackupVaultAPI{}
		c := govFakeClient(t, backupVaultCR())
		r := &BackupVaultReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateBackupVault")
		}
		got := &awsv1alpha1.BackupVault{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.VaultARN == "" {
			t.Error("status.vaultArn not populated")
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete removes vault", func(t *testing.T) {
		f := &fakeBackupVaultAPI{exists: true}
		c := govFakeClient(t, backupVaultCR(func(v *awsv1alpha1.BackupVault) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VaultARN = "arn:aws:backup:us-east-1:123456789012:backup-vault:my-vault"
		}))
		if err := c.Delete(ctx, backupVaultCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &BackupVaultReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteBackupVault")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.BackupVault{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		f := &fakeBackupVaultAPI{exists: true}
		c := govFakeClient(t, backupVaultCR(func(v *awsv1alpha1.BackupVault) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		if err := c.Delete(ctx, backupVaultCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &BackupVaultReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteBackupVault must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.BackupVault{})
	})
}

// -------------------------------------------------------------- BackupSelection

type fakeBackupSelectionAPI struct {
	createCalled bool
	deleteCalled bool
	deletedSelID string
}

func (f *fakeBackupSelectionAPI) GetBackupSelection(_ context.Context, _ *awsbackup.GetBackupSelectionInput, _ ...func(*awsbackup.Options)) (*awsbackup.GetBackupSelectionOutput, error) {
	return &awsbackup.GetBackupSelectionOutput{}, nil
}

func (f *fakeBackupSelectionAPI) CreateBackupSelection(_ context.Context, params *awsbackup.CreateBackupSelectionInput, _ ...func(*awsbackup.Options)) (*awsbackup.CreateBackupSelectionOutput, error) {
	f.createCalled = true
	if aws.ToString(params.BackupSelection.IamRoleArn) == "" {
		return nil, fmt.Errorf("missing IAM role ARN")
	}
	return &awsbackup.CreateBackupSelectionOutput{
		SelectionId:  aws.String("sel-123"),
		BackupPlanId: params.BackupPlanId,
	}, nil
}

func (f *fakeBackupSelectionAPI) DeleteBackupSelection(_ context.Context, params *awsbackup.DeleteBackupSelectionInput, _ ...func(*awsbackup.Options)) (*awsbackup.DeleteBackupSelectionOutput, error) {
	f.deleteCalled = true
	f.deletedSelID = aws.ToString(params.SelectionId)
	return &awsbackup.DeleteBackupSelectionOutput{}, nil
}

func backupSelectionCR(mutate ...func(*awsv1alpha1.BackupSelection)) *awsv1alpha1.BackupSelection {
	sel := &awsv1alpha1.BackupSelection{
		ObjectMeta: metav1.ObjectMeta{Name: "my-selection", Namespace: "default"},
		Spec: awsv1alpha1.BackupSelectionSpec{
			PlanRef:       "my-plan",
			SelectionName: "my-selection",
			IAMRoleARN:    "arn:aws:iam::123456789012:role/backup-role",
			Resources:     []string{"arn:aws:dynamodb:*:*:table/*"},
		},
	}
	for _, m := range mutate {
		m(sel)
	}
	return sel
}

func TestBackupSelectionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-selection", Namespace: "default"}}
	ctx := context.Background()

	planWithID := backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
		p.Status.PlanID = testPlanID
	})

	t.Run("create persists selection ID and Ready", func(t *testing.T) {
		f := &fakeBackupSelectionAPI{}
		c := govFakeClient(t, backupSelectionCR(), planWithID.DeepCopy())
		r := &BackupSelectionReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateBackupSelection")
		}
		got := &awsv1alpha1.BackupSelection{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.SelectionID != "sel-123" {
			t.Errorf("status.selectionId = %q, want sel-123", got.Status.SelectionID)
		}
		if got.Status.PlanID != testPlanID {
			t.Errorf("status.planId = %q, want %q", got.Status.PlanID, testPlanID)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("plan without ID requeues as dependency", func(t *testing.T) {
		f := &fakeBackupSelectionAPI{}
		c := govFakeClient(t, backupSelectionCR(), backupPlanCR())
		r := &BackupSelectionReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("CreateBackupSelection must not be called while plan is not ready")
		}
	})

	t.Run("delete removes selection", func(t *testing.T) {
		f := &fakeBackupSelectionAPI{}
		c := govFakeClient(t, backupSelectionCR(func(s *awsv1alpha1.BackupSelection) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Status.SelectionID = "sel-123"
			s.Status.PlanID = testPlanID
		}))
		if err := c.Delete(ctx, backupSelectionCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &BackupSelectionReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedSelID != "sel-123" {
			t.Errorf("expected DeleteBackupSelection with sel-123, called=%v id=%q", f.deleteCalled, f.deletedSelID)
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.BackupSelection{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		f := &fakeBackupSelectionAPI{}
		c := govFakeClient(t, backupSelectionCR(func(s *awsv1alpha1.BackupSelection) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			s.Status.SelectionID = "sel-123"
			s.Status.PlanID = testPlanID
		}))
		if err := c.Delete(ctx, backupSelectionCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &BackupSelectionReconciler{Client: c, Scheme: newGovScheme(t), BackupClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteBackupSelection must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.BackupSelection{})
	})
}

// ------------------------------------------------------------ GuardDutyDetector

type fakeGuardDutyAPI struct {
	detectorIDs  []string
	createCalled bool
	deleteCalled bool
	deletedID    string
}

func (f *fakeGuardDutyAPI) ListDetectors(_ context.Context, _ *awsguardduty.ListDetectorsInput, _ ...func(*awsguardduty.Options)) (*awsguardduty.ListDetectorsOutput, error) {
	return &awsguardduty.ListDetectorsOutput{DetectorIds: f.detectorIDs}, nil
}

func (f *fakeGuardDutyAPI) GetDetector(_ context.Context, _ *awsguardduty.GetDetectorInput, _ ...func(*awsguardduty.Options)) (*awsguardduty.GetDetectorOutput, error) {
	return &awsguardduty.GetDetectorOutput{Status: "ENABLED", ServiceRole: aws.String("role")}, nil
}

func (f *fakeGuardDutyAPI) CreateDetector(_ context.Context, _ *awsguardduty.CreateDetectorInput, _ ...func(*awsguardduty.Options)) (*awsguardduty.CreateDetectorOutput, error) {
	f.createCalled = true
	return &awsguardduty.CreateDetectorOutput{DetectorId: aws.String("det-123")}, nil
}

func (f *fakeGuardDutyAPI) UpdateDetector(_ context.Context, _ *awsguardduty.UpdateDetectorInput, _ ...func(*awsguardduty.Options)) (*awsguardduty.UpdateDetectorOutput, error) {
	return &awsguardduty.UpdateDetectorOutput{}, nil
}

func (f *fakeGuardDutyAPI) DeleteDetector(_ context.Context, params *awsguardduty.DeleteDetectorInput, _ ...func(*awsguardduty.Options)) (*awsguardduty.DeleteDetectorOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.DetectorId)
	return &awsguardduty.DeleteDetectorOutput{}, nil
}

func guardDutyCR(mutate ...func(*awsv1alpha1.GuardDutyDetector)) *awsv1alpha1.GuardDutyDetector {
	det := &awsv1alpha1.GuardDutyDetector{
		ObjectMeta: metav1.ObjectMeta{Name: "detector", Namespace: "default"},
		Spec: awsv1alpha1.GuardDutyDetectorSpec{
			FindingPublishingFrequency: "SIX_HOURS",
		},
	}
	for _, m := range mutate {
		m(det)
	}
	return det
}

func TestGuardDutyDetectorReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "detector", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create persists detector ID and Ready", func(t *testing.T) {
		f := &fakeGuardDutyAPI{}
		c := govFakeClient(t, guardDutyCR())
		r := &GuardDutyDetectorReconciler{Client: c, Scheme: newGovScheme(t), GuardDutyClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateDetector")
		}
		got := &awsv1alpha1.GuardDutyDetector{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.DetectorID != "det-123" {
			t.Errorf("status.detectorId = %q, want det-123", got.Status.DetectorID)
		}
		if got.Status.DetectorStatus != "ENABLED" {
			t.Errorf("status.detectorStatus = %q, want ENABLED", got.Status.DetectorStatus)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("adopts existing detector instead of creating", func(t *testing.T) {
		f := &fakeGuardDutyAPI{detectorIDs: []string{"existing-det"}}
		c := govFakeClient(t, guardDutyCR())
		r := &GuardDutyDetectorReconciler{Client: c, Scheme: newGovScheme(t), GuardDutyClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("CreateDetector must not be called when a detector already exists")
		}
		got := &awsv1alpha1.GuardDutyDetector{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.DetectorID != "existing-det" {
			t.Errorf("status.detectorId = %q, want existing-det", got.Status.DetectorID)
		}
	})

	t.Run("delete removes detector", func(t *testing.T) {
		f := &fakeGuardDutyAPI{}
		c := govFakeClient(t, guardDutyCR(func(d *awsv1alpha1.GuardDutyDetector) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Status.DetectorID = "det-123"
		}))
		if err := c.Delete(ctx, guardDutyCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &GuardDutyDetectorReconciler{Client: c, Scheme: newGovScheme(t), GuardDutyClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedID != "det-123" {
			t.Errorf("expected DeleteDetector with det-123, called=%v id=%q", f.deleteCalled, f.deletedID)
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.GuardDutyDetector{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		f := &fakeGuardDutyAPI{}
		c := govFakeClient(t, guardDutyCR(func(d *awsv1alpha1.GuardDutyDetector) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			d.Status.DetectorID = "det-123"
		}))
		if err := c.Delete(ctx, guardDutyCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &GuardDutyDetectorReconciler{Client: c, Scheme: newGovScheme(t), GuardDutyClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteDetector must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.GuardDutyDetector{})
	})
}

// ----------------------------------------------------------- SecurityHubAccount

type fakeSecurityHubAccountAPI struct {
	enabled       bool
	enableCalled  bool
	disableCalled bool
}

func (f *fakeSecurityHubAccountAPI) DescribeHub(_ context.Context, _ *awssecurityhub.DescribeHubInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.DescribeHubOutput, error) {
	if !f.enabled {
		return nil, &smithy.GenericAPIError{Code: "InvalidAccessException", Message: "not subscribed to Security Hub"}
	}
	return &awssecurityhub.DescribeHubOutput{
		HubArn:       aws.String("arn:aws:securityhub:us-east-1:123456789012:hub/default"),
		SubscribedAt: aws.String("2026-01-01T00:00:00.000Z"),
	}, nil
}

func (f *fakeSecurityHubAccountAPI) EnableSecurityHub(_ context.Context, _ *awssecurityhub.EnableSecurityHubInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.EnableSecurityHubOutput, error) {
	f.enableCalled = true
	f.enabled = true
	return &awssecurityhub.EnableSecurityHubOutput{}, nil
}

func (f *fakeSecurityHubAccountAPI) DisableSecurityHub(_ context.Context, _ *awssecurityhub.DisableSecurityHubInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.DisableSecurityHubOutput, error) {
	f.disableCalled = true
	f.enabled = false
	return &awssecurityhub.DisableSecurityHubOutput{}, nil
}

func (f *fakeSecurityHubAccountAPI) UpdateSecurityHubConfiguration(_ context.Context, _ *awssecurityhub.UpdateSecurityHubConfigurationInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.UpdateSecurityHubConfigurationOutput, error) {
	return &awssecurityhub.UpdateSecurityHubConfigurationOutput{}, nil
}

func securityHubAccountCR(mutate ...func(*awsv1alpha1.SecurityHubAccount)) *awsv1alpha1.SecurityHubAccount {
	hub := &awsv1alpha1.SecurityHubAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "hub", Namespace: "default"},
		Spec: awsv1alpha1.SecurityHubAccountSpec{
			EnableDefaultStandards: aws.Bool(false),
		},
	}
	for _, m := range mutate {
		m(hub)
	}
	return hub
}

func TestSecurityHubAccountReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "hub", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create enables hub and persists ARN", func(t *testing.T) {
		f := &fakeSecurityHubAccountAPI{}
		c := govFakeClient(t, securityHubAccountCR())
		r := &SecurityHubAccountReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.enableCalled {
			t.Error("expected EnableSecurityHub")
		}
		got := &awsv1alpha1.SecurityHubAccount{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.HubARN == "" {
			t.Error("status.hubArn not populated")
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete disables hub", func(t *testing.T) {
		f := &fakeSecurityHubAccountAPI{enabled: true}
		c := govFakeClient(t, securityHubAccountCR(func(h *awsv1alpha1.SecurityHubAccount) {
			h.Finalizers = []string{awsv1alpha1.FinalizerName}
		}))
		if err := c.Delete(ctx, securityHubAccountCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &SecurityHubAccountReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.disableCalled {
			t.Error("expected DisableSecurityHub")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.SecurityHubAccount{})
	})

	t.Run("abandon skips disable", func(t *testing.T) {
		f := &fakeSecurityHubAccountAPI{enabled: true}
		c := govFakeClient(t, securityHubAccountCR(func(h *awsv1alpha1.SecurityHubAccount) {
			h.Finalizers = []string{awsv1alpha1.FinalizerName}
			h.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		if err := c.Delete(ctx, securityHubAccountCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &SecurityHubAccountReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.disableCalled {
			t.Error("DisableSecurityHub must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.SecurityHubAccount{})
	})
}

// ---------------------------------------------------------- SecurityHubStandard

const testStandardsARN = "arn:aws:securityhub:us-east-1::standards/aws-foundational-security-best-practices/v/1.0.0"
const testSubscriptionARN = "arn:aws:securityhub:us-east-1:123456789012:subscription/aws-foundational-security-best-practices/v/1.0.0"

type fakeSecurityHubStandardAPI struct {
	enabled       bool
	enableCalled  bool
	disableCalled bool
	disabledARNs  []string
}

func (f *fakeSecurityHubStandardAPI) GetEnabledStandards(_ context.Context, _ *awssecurityhub.GetEnabledStandardsInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.GetEnabledStandardsOutput, error) {
	out := &awssecurityhub.GetEnabledStandardsOutput{}
	if f.enabled {
		out.StandardsSubscriptions = []securityhubtypes.StandardsSubscription{{
			StandardsArn:             aws.String(testStandardsARN),
			StandardsSubscriptionArn: aws.String(testSubscriptionARN),
			StandardsStatus:          securityhubtypes.StandardsStatusReady,
		}}
	}
	return out, nil
}

func (f *fakeSecurityHubStandardAPI) BatchEnableStandards(_ context.Context, params *awssecurityhub.BatchEnableStandardsInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.BatchEnableStandardsOutput, error) {
	f.enableCalled = true
	f.enabled = true
	return &awssecurityhub.BatchEnableStandardsOutput{
		StandardsSubscriptions: []securityhubtypes.StandardsSubscription{{
			StandardsArn:             params.StandardsSubscriptionRequests[0].StandardsArn,
			StandardsSubscriptionArn: aws.String(testSubscriptionARN),
			StandardsStatus:          securityhubtypes.StandardsStatusPending,
		}},
	}, nil
}

func (f *fakeSecurityHubStandardAPI) BatchDisableStandards(_ context.Context, params *awssecurityhub.BatchDisableStandardsInput, _ ...func(*awssecurityhub.Options)) (*awssecurityhub.BatchDisableStandardsOutput, error) {
	f.disableCalled = true
	f.disabledARNs = params.StandardsSubscriptionArns
	return &awssecurityhub.BatchDisableStandardsOutput{}, nil
}

func securityHubStandardCR(mutate ...func(*awsv1alpha1.SecurityHubStandard)) *awsv1alpha1.SecurityHubStandard {
	std := &awsv1alpha1.SecurityHubStandard{
		ObjectMeta: metav1.ObjectMeta{Name: "fsbp", Namespace: "default"},
		Spec:       awsv1alpha1.SecurityHubStandardSpec{StandardsARN: testStandardsARN},
	}
	for _, m := range mutate {
		m(std)
	}
	return std
}

func TestSecurityHubStandardReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "fsbp", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create enables standard and persists subscription ARN", func(t *testing.T) {
		f := &fakeSecurityHubStandardAPI{}
		c := govFakeClient(t, securityHubStandardCR())
		r := &SecurityHubStandardReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.enableCalled {
			t.Error("expected BatchEnableStandards")
		}
		got := &awsv1alpha1.SecurityHubStandard{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.SubscriptionARN != testSubscriptionARN {
			t.Errorf("status.subscriptionArn = %q, want %q", got.Status.SubscriptionARN, testSubscriptionARN)
		}
		if got.Status.StandardsStatus != "PENDING" {
			t.Errorf("status.standardsStatus = %q, want PENDING", got.Status.StandardsStatus)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("already enabled does not re-enable", func(t *testing.T) {
		f := &fakeSecurityHubStandardAPI{enabled: true}
		c := govFakeClient(t, securityHubStandardCR())
		r := &SecurityHubStandardReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.enableCalled {
			t.Error("BatchEnableStandards must not be called when already enabled")
		}
		got := &awsv1alpha1.SecurityHubStandard{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.StandardsStatus != "READY" {
			t.Errorf("status.standardsStatus = %q, want READY", got.Status.StandardsStatus)
		}
	})

	t.Run("delete disables standard", func(t *testing.T) {
		f := &fakeSecurityHubStandardAPI{enabled: true}
		c := govFakeClient(t, securityHubStandardCR(func(s *awsv1alpha1.SecurityHubStandard) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Status.SubscriptionARN = testSubscriptionARN
		}))
		if err := c.Delete(ctx, securityHubStandardCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &SecurityHubStandardReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.disableCalled || len(f.disabledARNs) != 1 || f.disabledARNs[0] != testSubscriptionARN {
			t.Errorf("expected BatchDisableStandards with %q, got called=%v arns=%v", testSubscriptionARN, f.disableCalled, f.disabledARNs)
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.SecurityHubStandard{})
	})

	t.Run("abandon skips disable", func(t *testing.T) {
		f := &fakeSecurityHubStandardAPI{enabled: true}
		c := govFakeClient(t, securityHubStandardCR(func(s *awsv1alpha1.SecurityHubStandard) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			s.Status.SubscriptionARN = testSubscriptionARN
		}))
		if err := c.Delete(ctx, securityHubStandardCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &SecurityHubStandardReconciler{Client: c, Scheme: newGovScheme(t), SecurityHubClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.disableCalled {
			t.Error("BatchDisableStandards must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.SecurityHubStandard{})
	})
}

// ------------------------------------------------------------- InspectorEnabler

type fakeInspectorAPI struct {
	ec2Enabled     bool
	ecrEnabled     bool
	enableCalled   bool
	enabledTypes   []inspector2types.ResourceScanType
	disableCalled  bool
	disabledTypes  []inspector2types.ResourceScanType
	statusCallsErr error
}

func (f *fakeInspectorAPI) state(enabled bool) *inspector2types.State {
	st := inspector2types.StatusDisabled
	if enabled {
		st = inspector2types.StatusEnabled
	}
	return &inspector2types.State{Status: st}
}

func (f *fakeInspectorAPI) BatchGetAccountStatus(_ context.Context, _ *awsinspector2.BatchGetAccountStatusInput, _ ...func(*awsinspector2.Options)) (*awsinspector2.BatchGetAccountStatusOutput, error) {
	if f.statusCallsErr != nil {
		return nil, f.statusCallsErr
	}
	overall := inspector2types.StatusDisabled
	if f.ec2Enabled || f.ecrEnabled {
		overall = inspector2types.StatusEnabled
	}
	return &awsinspector2.BatchGetAccountStatusOutput{
		Accounts: []inspector2types.AccountState{{
			AccountId: aws.String("123456789012"),
			State:     &inspector2types.State{Status: overall},
			ResourceState: &inspector2types.ResourceState{
				Ec2:    f.state(f.ec2Enabled),
				Ecr:    f.state(f.ecrEnabled),
				Lambda: f.state(false),
			},
		}},
	}, nil
}

func (f *fakeInspectorAPI) Enable(_ context.Context, params *awsinspector2.EnableInput, _ ...func(*awsinspector2.Options)) (*awsinspector2.EnableOutput, error) {
	f.enableCalled = true
	f.enabledTypes = params.ResourceTypes
	for _, rt := range params.ResourceTypes {
		switch rt {
		case inspector2types.ResourceScanTypeEc2:
			f.ec2Enabled = true
		case inspector2types.ResourceScanTypeEcr:
			f.ecrEnabled = true
		}
	}
	return &awsinspector2.EnableOutput{}, nil
}

func (f *fakeInspectorAPI) Disable(_ context.Context, params *awsinspector2.DisableInput, _ ...func(*awsinspector2.Options)) (*awsinspector2.DisableOutput, error) {
	f.disableCalled = true
	f.disabledTypes = params.ResourceTypes
	return &awsinspector2.DisableOutput{}, nil
}

func inspectorCR(mutate ...func(*awsv1alpha1.InspectorEnabler)) *awsv1alpha1.InspectorEnabler {
	ins := &awsv1alpha1.InspectorEnabler{
		ObjectMeta: metav1.ObjectMeta{Name: "inspector", Namespace: "default"},
		Spec:       awsv1alpha1.InspectorEnablerSpec{ResourceTypes: []awsv1alpha1.InspectorResourceType{"EC2", "ECR"}},
	}
	for _, m := range mutate {
		m(ins)
	}
	return ins
}

func TestInspectorEnablerReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "inspector", Namespace: "default"}}
	ctx := context.Background()

	t.Run("create enables missing scan types and reports status", func(t *testing.T) {
		f := &fakeInspectorAPI{ec2Enabled: true} // ECR still disabled
		c := govFakeClient(t, inspectorCR())
		r := &InspectorEnablerReconciler{Client: c, Scheme: newGovScheme(t), InspectorClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.enableCalled {
			t.Error("expected Enable to be called")
		}
		if len(f.enabledTypes) != 1 || f.enabledTypes[0] != inspector2types.ResourceScanTypeEcr {
			t.Errorf("Enable types = %v, want [ECR] only (EC2 already enabled)", f.enabledTypes)
		}
		got := &awsv1alpha1.InspectorEnabler{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.AccountID != "123456789012" {
			t.Errorf("status.accountId = %q, want 123456789012", got.Status.AccountID)
		}
		if got.Status.ResourceStatus == nil || got.Status.ResourceStatus.ECR != "ENABLED" {
			t.Errorf("status.resourceStatus = %+v, want ECR ENABLED", got.Status.ResourceStatus)
		}
		assertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("steady state does not re-enable", func(t *testing.T) {
		f := &fakeInspectorAPI{ec2Enabled: true, ecrEnabled: true}
		c := govFakeClient(t, inspectorCR(func(i *awsv1alpha1.InspectorEnabler) {
			i.Finalizers = []string{awsv1alpha1.FinalizerName}
		}))
		r := &InspectorEnablerReconciler{Client: c, Scheme: newGovScheme(t), InspectorClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.enableCalled {
			t.Error("Enable must not be called when all scan types are enabled")
		}
	})

	t.Run("delete disables managed scan types", func(t *testing.T) {
		f := &fakeInspectorAPI{ec2Enabled: true, ecrEnabled: true}
		c := govFakeClient(t, inspectorCR(func(i *awsv1alpha1.InspectorEnabler) {
			i.Finalizers = []string{awsv1alpha1.FinalizerName}
		}))
		if err := c.Delete(ctx, inspectorCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &InspectorEnablerReconciler{Client: c, Scheme: newGovScheme(t), InspectorClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.disableCalled || len(f.disabledTypes) != 2 {
			t.Errorf("expected Disable with 2 types, got called=%v types=%v", f.disableCalled, f.disabledTypes)
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.InspectorEnabler{})
	})

	t.Run("abandon skips disable", func(t *testing.T) {
		f := &fakeInspectorAPI{ec2Enabled: true, ecrEnabled: true}
		c := govFakeClient(t, inspectorCR(func(i *awsv1alpha1.InspectorEnabler) {
			i.Finalizers = []string{awsv1alpha1.FinalizerName}
			i.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		if err := c.Delete(ctx, inspectorCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &InspectorEnablerReconciler{Client: c, Scheme: newGovScheme(t), InspectorClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.disableCalled {
			t.Error("Disable must not be called when abandoning")
		}
		assertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.InspectorEnabler{})
	})
}
