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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
	"github.com/konfig-io/konfig-konector/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(awsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		// Production-mode structured JSON logging by default; override with
		// --zap-devel for local development.
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "d87b8e36.konfig.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("initialising AWS clients")
	awsClients, err := awsclient.NewClients(context.Background())
	if err != nil {
		setupLog.Error(err, "unable to initialise AWS clients")
		os.Exit(1)
	}

	// Multi-account: every reconcile resolves its AWSProvider (spec.providerRef,
	// namespace annotation, or operator default) and the SDK wrappers apply
	// the resulting credentials/region per call.
	controller.SetProviderResolver(&provider.Resolver{
		Client:          mgr.GetClient(),
		STS:             awsClients.STS,
		BaseCredentials: awsClients.Config.Credentials,
		BaseRegion:      awsClients.Config.Region,
	})

	// Use the operator pod name (or a fixed string) as the caller reference prefix
	// for Route53 idempotency tokens.
	callerRef := os.Getenv("POD_NAME")
	if callerRef == "" {
		callerRef = "konfig-konector"
	}

	if err = (&controller.IAMRoleReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMRole")
		os.Exit(1)
	}
	if err = (&controller.IAMPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMPolicy")
		os.Exit(1)
	}
	if err = (&controller.IAMPolicyAttachmentReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMPolicyAttachment")
		os.Exit(1)
	}
	if err = (&controller.IAMRolePolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMRolePolicy")
		os.Exit(1)
	}
	if err = (&controller.PodIdentityAssociationReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PodIdentityAssociation")
		os.Exit(1)
	}
	if err = (&controller.HostedZoneReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Route53Client:   awsClients.Route53,
		CallerReference: callerRef,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HostedZone")
		os.Exit(1)
	}
	if err = (&controller.RecordSetReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Route53Client: awsClients.Route53,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RecordSet")
		os.Exit(1)
	}
	if err = (&controller.HealthCheckReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Route53Client:   awsClients.Route53,
		CallerReference: callerRef,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HealthCheck")
		os.Exit(1)
	}
	if err = (&controller.VPCReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VPC")
		os.Exit(1)
	}
	if err = (&controller.SubnetReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Subnet")
		os.Exit(1)
	}
	if err = (&controller.InternetGatewayReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "InternetGateway")
		os.Exit(1)
	}
	if err = (&controller.RouteTableReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RouteTable")
		os.Exit(1)
	}
	if err = (&controller.NatGatewayReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NatGateway")
		os.Exit(1)
	}
	if err = (&controller.SecurityGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SecurityGroup")
		os.Exit(1)
	}
	if err = (&controller.VPCEndpointReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VPCEndpoint")
		os.Exit(1)
	}
	if err = (&controller.DBSubnetGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBSubnetGroup")
		os.Exit(1)
	}
	if err = (&controller.DBParameterGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBParameterGroup")
		os.Exit(1)
	}
	if err = (&controller.DBClusterParameterGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBClusterParameterGroup")
		os.Exit(1)
	}
	if err = (&controller.DBInstanceReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBInstance")
		os.Exit(1)
	}
	if err = (&controller.DBClusterReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBCluster")
		os.Exit(1)
	}
	if err = (&controller.KeyPairReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeyPair")
		os.Exit(1)
	}
	if err = (&controller.LaunchTemplateReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LaunchTemplate")
		os.Exit(1)
	}
	if err = (&controller.AutoScalingGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ASGClient: awsClients.AutoScaling,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AutoScalingGroup")
		os.Exit(1)
	}
	if err = (&controller.S3BucketReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: awsClients.S3,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3Bucket")
		os.Exit(1)
	}
	if err = (&controller.S3BucketPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: awsClients.S3,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3BucketPolicy")
		os.Exit(1)
	}
	if err = (&controller.SQSQueueReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SQSClient: awsClients.SQS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SQSQueue")
		os.Exit(1)
	}
	if err = (&controller.SNSTopicReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SNSClient: awsClients.SNS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SNSTopic")
		os.Exit(1)
	}
	if err = (&controller.SNSSubscriptionReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SNSClient: awsClients.SNS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SNSSubscription")
		os.Exit(1)
	}
	if err = (&controller.ElastiCacheSubnetGroupReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		ElastiCacheClient: awsClients.ElastiCache,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ElastiCacheSubnetGroup")
		os.Exit(1)
	}
	if err = (&controller.ElastiCacheReplicationGroupReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		ElastiCacheClient: awsClients.ElastiCache,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ElastiCacheReplicationGroup")
		os.Exit(1)
	}
	if err = (&controller.EC2InstanceReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EC2Instance")
		os.Exit(1)
	}
	if err = (&controller.EKSClusterReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSCluster")
		os.Exit(1)
	}
	if err = (&controller.EKSNodeGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSNodeGroup")
		os.Exit(1)
	}
	if err = (&controller.EKSAddonReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSAddon")
		os.Exit(1)
	}
	if err = (&controller.EKSFargateProfileReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSFargateProfile")
		os.Exit(1)
	}
	if err = (&controller.EKSAccessEntryReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSAccessEntry")
		os.Exit(1)
	}
	if err = (&controller.LambdaFunctionReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaFunction")
		os.Exit(1)
	}
	if err = (&controller.LambdaEventSourceMappingReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaEventSourceMapping")
		os.Exit(1)
	}
	if err = (&controller.LambdaPermissionReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaPermission")
		os.Exit(1)
	}
	if err = (&controller.ECSClusterReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECSClient: awsClients.ECS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECSCluster")
		os.Exit(1)
	}
	if err = (&controller.ECSTaskDefinitionReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECSClient: awsClients.ECS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECSTaskDefinition")
		os.Exit(1)
	}
	if err = (&controller.ECSServiceReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECSClient: awsClients.ECS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECSService")
		os.Exit(1)
	}
	if err = (&controller.IAMUserReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMUser")
		os.Exit(1)
	}
	if err = (&controller.IAMGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMGroup")
		os.Exit(1)
	}
	if err = (&controller.IAMGroupPolicyAttachmentReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMGroupPolicyAttachment")
		os.Exit(1)
	}
	if err = (&controller.IAMGroupMembershipReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMGroupMembership")
		os.Exit(1)
	}
	if err = (&controller.IAMSAMLProviderReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMSAMLProvider")
		os.Exit(1)
	}
	if err = (&controller.IAMOIDCProviderReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		IAMClient: awsClients.IAM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IAMOIDCProvider")
		os.Exit(1)
	}
	if err = (&controller.ElasticIPReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ElasticIP")
		os.Exit(1)
	}
	if err = (&controller.EIPAssociationReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EIPAssociation")
		os.Exit(1)
	}
	if err = (&controller.NetworkACLReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NetworkACL")
		os.Exit(1)
	}
	if err = (&controller.PlacementGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PlacementGroup")
		os.Exit(1)
	}
	if err = (&controller.TransitGatewayReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TransitGateway")
		os.Exit(1)
	}
	if err = (&controller.TransitGatewayVpcAttachmentReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TransitGatewayVpcAttachment")
		os.Exit(1)
	}
	if err = (&controller.DBOptionGroupReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBOptionGroup")
		os.Exit(1)
	}
	if err = (&controller.ScalingPolicyReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		AutoScalingClient: awsClients.AutoScaling,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScalingPolicy")
		os.Exit(1)
	}
	if err = (&controller.CertificateReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ACMClient: awsClients.ACM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Certificate")
		os.Exit(1)
	}
	if err = (&controller.CertificateValidationReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ACMClient: awsClients.ACM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CertificateValidation")
		os.Exit(1)
	}
	if err = (&controller.LoadBalancerReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ELBv2Client: awsClients.ELBv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LoadBalancer")
		os.Exit(1)
	}
	if err = (&controller.TargetGroupReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ELBv2Client: awsClients.ELBv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TargetGroup")
		os.Exit(1)
	}
	if err = (&controller.ListenerReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ELBv2Client: awsClients.ELBv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Listener")
		os.Exit(1)
	}
	if err = (&controller.ListenerRuleReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ELBv2Client: awsClients.ELBv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ListenerRule")
		os.Exit(1)
	}
	if err = (&controller.ECRRepositoryReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECRClient: awsClients.ECR,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECRRepository")
		os.Exit(1)
	}
	if err = (&controller.ECRRepositoryPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECRClient: awsClients.ECR,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECRRepositoryPolicy")
		os.Exit(1)
	}
	if err = (&controller.ECRLifecyclePolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECRClient: awsClients.ECR,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECRLifecyclePolicy")
		os.Exit(1)
	}
	if err = (&controller.LogGroupReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		LogsClient: awsClients.CloudWatchLogs,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LogGroup")
		os.Exit(1)
	}
	if err = (&controller.MetricFilterReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		LogsClient: awsClients.CloudWatchLogs,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MetricFilter")
		os.Exit(1)
	}
	if err = (&controller.SubscriptionFilterReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		LogsClient: awsClients.CloudWatchLogs,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SubscriptionFilter")
		os.Exit(1)
	}
	if err = (&controller.KMSKeyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		KMSClient: awsClients.KMS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KMSKey")
		os.Exit(1)
	}
	if err = (&controller.KMSAliasReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		KMSClient: awsClients.KMS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KMSAlias")
		os.Exit(1)
	}
	if err = (&controller.KMSKeyPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		KMSClient: awsClients.KMS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KMSKeyPolicy")
		os.Exit(1)
	}
	if err = (&controller.SecretReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		SecretsManagerClient: awsClients.SecretsManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Secret")
		os.Exit(1)
	}
	if err = (&controller.SecretRotationReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		SecretsManagerClient: awsClients.SecretsManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SecretRotation")
		os.Exit(1)
	}
	if err = (&controller.EventBusReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventBridgeClient: awsClients.EventBridge,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EventBus")
		os.Exit(1)
	}
	if err = (&controller.EventRuleReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventBridgeClient: awsClients.EventBridge,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EventRule")
		os.Exit(1)
	}
	if err = (&controller.EventTargetReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventBridgeClient: awsClients.EventBridge,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EventTarget")
		os.Exit(1)
	}
	if err = (&controller.DynamoDBTableReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		DynamoDBClient: awsClients.DynamoDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DynamoDBTable")
		os.Exit(1)
	}
	if err = (&controller.DynamoDBGlobalTableReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		DynamoDBClient: awsClients.DynamoDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DynamoDBGlobalTable")
		os.Exit(1)
	}
	if err = (&controller.UserPoolReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		CognitoClient: awsClients.Cognito,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserPool")
		os.Exit(1)
	}
	if err = (&controller.UserPoolClientReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		CognitoClient: awsClients.Cognito,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserPoolClient")
		os.Exit(1)
	}
	if err = (&controller.IdentityProviderReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		CognitoClient: awsClients.Cognito,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IdentityProvider")
		os.Exit(1)
	}
	if err = (&controller.WebACLReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		WAFv2Client: awsClients.WAFv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebACL")
		os.Exit(1)
	}
	if err = (&controller.IPSetReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		WAFv2Client: awsClients.WAFv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPSet")
		os.Exit(1)
	}
	if err = (&controller.SSMParameterReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SSMClient: awsClients.SSM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SSMParameter")
		os.Exit(1)
	}
	if err = (&controller.VPCPeeringConnectionReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VPCPeeringConnection")
		os.Exit(1)
	}
	if err = (&controller.EgressOnlyIGWReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EgressOnlyIGW")
		os.Exit(1)
	}
	if err = (&controller.FlowLogReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FlowLog")
		os.Exit(1)
	}
	if err = (&controller.EBSVolumeReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EBSVolume")
		os.Exit(1)
	}
	if err = (&controller.SpotFleetReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SpotFleet")
		os.Exit(1)
	}
	if err = (&controller.AMIReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: awsClients.EC2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AMI")
		os.Exit(1)
	}
	if err = (&controller.LambdaFunctionURLReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaFunctionURL")
		os.Exit(1)
	}
	if err = (&controller.LambdaCodeSigningConfigReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaCodeSigningConfig")
		os.Exit(1)
	}
	if err = (&controller.LambdaAliasReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaAlias")
		os.Exit(1)
	}
	if err = (&controller.LambdaLayerVersionReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		LambdaClient: awsClients.Lambda,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LambdaLayerVersion")
		os.Exit(1)
	}
	if err = (&controller.DAXClusterReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		DAXClient: awsClients.DAX,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DAXCluster")
		os.Exit(1)
	}
	if err = (&controller.EKSIdentityProviderConfigReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EKSClient: awsClients.EKS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSIdentityProviderConfig")
		os.Exit(1)
	}
	if err = (&controller.ElastiCacheServerlessCacheReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		ElastiCacheClient: awsClients.ElastiCache,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ElastiCacheServerlessCache")
		os.Exit(1)
	}
	if err = (&controller.MemoryDBClusterReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		MemoryDBClient: awsClients.MemoryDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MemoryDBCluster")
		os.Exit(1)
	}
	if err = (&controller.EFSAccessPointReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EFSClient: awsClients.EFS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EFSAccessPoint")
		os.Exit(1)
	}
	if err = (&controller.ElastiCacheParameterGroupReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		ElastiCacheClient: awsClients.ElastiCache,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ElastiCacheParameterGroup")
		os.Exit(1)
	}
	if err = (&controller.EFSFileSystemReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EFSClient: awsClients.EFS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EFSFileSystem")
		os.Exit(1)
	}
	if err = (&controller.EFSMountTargetReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EFSClient: awsClients.EFS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EFSMountTarget")
		os.Exit(1)
	}
	if err = (&controller.S3BucketLifecycleReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: awsClients.S3,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3BucketLifecycle")
		os.Exit(1)
	}
	if err = (&controller.S3BucketReplicationReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: awsClients.S3,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3BucketReplication")
		os.Exit(1)
	}
	if err = (&controller.S3BucketNotificationReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: awsClients.S3,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3BucketNotification")
		os.Exit(1)
	}
	if err = (&controller.S3BucketCORSReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: awsClients.S3,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3BucketCORS")
		os.Exit(1)
	}
	if err = (&controller.DBSnapshotReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBSnapshot")
		os.Exit(1)
	}
	if err = (&controller.DBProxyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		RDSClient: awsClients.RDS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DBProxy")
		os.Exit(1)
	}
	if err = (&controller.KMSGrantReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		KMSClient: awsClients.KMS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KMSGrant")
		os.Exit(1)
	}
	if err = (&controller.SSMDocumentReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SSMClient: awsClients.SSM,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SSMDocument")
		os.Exit(1)
	}
	if err = (&controller.CloudFrontDistributionReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudFrontClient: awsClients.CloudFront,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudFrontDistribution")
		os.Exit(1)
	}
	if err = (&controller.CloudFrontOriginAccessControlReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudFrontClient: awsClients.CloudFront,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudFrontOriginAccessControl")
		os.Exit(1)
	}
	if err = (&controller.CloudFrontCachePolicyReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudFrontClient: awsClients.CloudFront,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudFrontCachePolicy")
		os.Exit(1)
	}
	if err = (&controller.CloudFrontFunctionReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudFrontClient: awsClients.CloudFront,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudFrontFunction")
		os.Exit(1)
	}
	if err = (&controller.SESEmailIdentityReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		SESv2Client: awsClients.SESv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SESEmailIdentity")
		os.Exit(1)
	}
	if err = (&controller.SESConfigurationSetReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		SESv2Client: awsClients.SESv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SESConfigurationSet")
		os.Exit(1)
	}
	if err = (&controller.EventBridgePipeReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		PipesClient: awsClients.Pipes,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EventBridgePipe")
		os.Exit(1)
	}
	if err = (&controller.CloudWatchAlarmReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudWatchClient: awsClients.CloudWatch,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudWatchAlarm")
		os.Exit(1)
	}
	if err = (&controller.CompositeAlarmReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudWatchClient: awsClients.CloudWatch,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CompositeAlarm")
		os.Exit(1)
	}
	if err = (&controller.CloudWatchDashboardReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CloudWatchClient: awsClients.CloudWatch,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudWatchDashboard")
		os.Exit(1)
	}
	if err = (&controller.ECSCapacityProviderReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		ECSClient: awsClients.ECS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECSCapacityProvider")
		os.Exit(1)
	}
	if err = (&controller.ECSScheduledTaskReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventBridgeClient: awsClients.EventBridge,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ECSScheduledTask")
		os.Exit(1)
	}
	if err = (&controller.ActivityReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SFNClient: awsClients.SFN,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Activity")
		os.Exit(1)
	}
	if err = (&controller.CodePipelineReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		CodePipelineClient: awsClients.CodePipeline,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CodePipeline")
		os.Exit(1)
	}
	if err = (&controller.CloudFormationStackSetReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		CloudFormationClient: awsClients.CloudFormation,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudFormationStackSet")
		os.Exit(1)
	}
	if err = (&controller.StateMachineReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		SFNClient: awsClients.SFN,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StateMachine")
		os.Exit(1)
	}
	if err = (&controller.DelegationSignerRecordReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Route53Client: awsClients.Route53,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DelegationSignerRecord")
		os.Exit(1)
	}
	if err = (&controller.CloudFormationStackReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		CloudFormationClient: awsClients.CloudFormation,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudFormationStack")
		os.Exit(1)
	}
	if err = (&controller.ResolverEndpointReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Route53ResolverClient: awsClients.Route53Resolver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ResolverEndpoint")
		os.Exit(1)
	}
	if err = (&controller.ResolverRuleReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Route53ResolverClient: awsClients.Route53Resolver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ResolverRule")
		os.Exit(1)
	}
	if err = (&controller.OpenSearchServerlessCollectionReconciler{
		Client:                     mgr.GetClient(),
		Scheme:                     mgr.GetScheme(),
		OpenSearchServerlessClient: awsClients.OpenSearchServerless,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenSearchServerlessCollection")
		os.Exit(1)
	}
	if err = (&controller.OpenSearchAccessPolicyReconciler{
		Client:                     mgr.GetClient(),
		Scheme:                     mgr.GetScheme(),
		OpenSearchServerlessClient: awsClients.OpenSearchServerless,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenSearchAccessPolicy")
		os.Exit(1)
	}
	if err = (&controller.MSKConfigurationReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		KafkaClient: awsClients.Kafka,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MSKConfiguration")
		os.Exit(1)
	}
	if err = (&controller.OpenSearchDomainReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		OpenSearchClient: awsClients.OpenSearch,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenSearchDomain")
		os.Exit(1)
	}
	if err = (&controller.MSKClusterReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		KafkaClient: awsClients.Kafka,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MSKCluster")
		os.Exit(1)
	}
	if err = (&controller.MSKServerlessClusterReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		KafkaClient: awsClients.Kafka,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MSKServerlessCluster")
		os.Exit(1)
	}
	if err = (&controller.KinesisStreamConsumerReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		KinesisClient: awsClients.Kinesis,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KinesisStreamConsumer")
		os.Exit(1)
	}
	if err = (&controller.FirehoseDeliveryStreamReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		FirehoseClient: awsClients.Firehose,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FirehoseDeliveryStream")
		os.Exit(1)
	}
	if err = (&controller.DynamoDBTablePolicyReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		DynamoDBClient: awsClients.DynamoDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DynamoDBTablePolicy")
		os.Exit(1)
	}
	if err = (&controller.KinesisStreamReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		KinesisClient: awsClients.Kinesis,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KinesisStream")
		os.Exit(1)
	}
	if err = (&controller.ShieldProtectionReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		ShieldClient: awsClients.Shield,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ShieldProtection")
		os.Exit(1)
	}
	if err = (&controller.DynamoDBBackupReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		DynamoDBClient: awsClients.DynamoDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DynamoDBBackup")
		os.Exit(1)
	}
	if err = (&controller.WAFRegexPatternSetReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		WAFv2Client: awsClients.WAFv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WAFRegexPatternSet")
		os.Exit(1)
	}
	if err = (&controller.WAFRuleGroupReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		WAFv2Client: awsClients.WAFv2,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WAFRuleGroup")
		os.Exit(1)
	}
	if err = (&controller.CodeBuildProjectReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		CodeBuildClient: awsClients.CodeBuild,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CodeBuildProject")
		os.Exit(1)
	}
	if err = (&controller.CodeDeployApplicationReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CodeDeployClient: awsClients.CodeDeploy,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CodeDeployApplication")
		os.Exit(1)
	}
	if err = (&controller.CodeDeployDeploymentGroupReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CodeDeployClient: awsClients.CodeDeploy,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CodeDeployDeploymentGroup")
		os.Exit(1)
	}
	if err = (&controller.CodeCommitRepositoryReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CodeCommitClient: awsClients.CodeCommit,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CodeCommitRepository")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	// Controller families added after the round-2 expansion register
	// themselves via controller.RegisterSetup (see internal/controller/setup.go)
	// instead of an inline block here.
	if err := controller.SetupRegistered(mgr, awsClients); err != nil {
		setupLog.Error(err, "unable to create registered controllers")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
