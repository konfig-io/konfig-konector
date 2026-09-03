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

package exporters

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsautoscaling "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	awselbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	export.Register(export.Exporter{Kind: "TargetGroup", Service: "elbv2", Order: 30, Fn: exportTargetGroups})
	export.Register(export.Exporter{Kind: "LoadBalancer", Service: "elbv2", Order: 31, Fn: exportLoadBalancers})
	export.Register(export.Exporter{Kind: "Listener", Service: "elbv2", Order: 32, Fn: exportListeners})
	export.Register(export.Exporter{Kind: "ListenerRule", Service: "elbv2", Order: 33, Fn: exportListenerRules})
	export.Register(export.Exporter{Kind: "AutoScalingGroup", Service: "autoscaling", Order: 34, Fn: exportAutoScalingGroups})
	export.Register(export.Exporter{Kind: "ScalingPolicy", Service: "autoscaling", Order: 35, Fn: exportScalingPolicies})
}

// elbTagMap fetches the tags of one ELBv2 resource as a spec tag map.
// Tag lookup failures are non-fatal: the resource is exported without tags.
func elbTagMap(ctx context.Context, clients *awsclient.Clients, arn string) map[string]string {
	out, err := clients.ELBv2.DescribeTags(ctx, &awselbv2.DescribeTagsInput{ResourceArns: []string{arn}})
	if err != nil || len(out.TagDescriptions) == 0 {
		return nil
	}
	m := map[string]string{}
	for _, t := range out.TagDescriptions[0].Tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportTargetGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awselbv2.NewDescribeTargetGroupsPaginator(clients.ELBv2, &awselbv2.DescribeTargetGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe target groups: %w", err)
		}
		for _, tg := range page.TargetGroups {
			arn := aws.ToString(tg.TargetGroupArn)
			name := aws.ToString(tg.TargetGroupName)
			cr := &awsv1alpha1.TargetGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.TargetGroupSpec{
					Name:       name,
					Protocol:   string(tg.Protocol),
					Port:       aws.ToInt32(tg.Port),
					TargetType: string(tg.TargetType),
					Tags:       elbTagMap(ctx, clients, arn),
				},
			}
			if vpcID := aws.ToString(tg.VpcId); vpcID != "" {
				ref := vpcRefFor(vpcID, opts)
				cr.Spec.VPCRef = &ref
			}
			cr.Spec.HealthCheck = &awsv1alpha1.TargetGroupHealthCheck{
				Protocol:           string(tg.HealthCheckProtocol),
				Port:               aws.ToString(tg.HealthCheckPort),
				Path:               aws.ToString(tg.HealthCheckPath),
				HealthyThreshold:   aws.ToInt32(tg.HealthyThresholdCount),
				UnhealthyThreshold: aws.ToInt32(tg.UnhealthyThresholdCount),
				IntervalSeconds:    aws.ToInt32(tg.HealthCheckIntervalSeconds),
			}
			opts.Index.Add(arn, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportLoadBalancers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awselbv2.NewDescribeLoadBalancersPaginator(clients.ELBv2, &awselbv2.DescribeLoadBalancersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range page.LoadBalancers {
			arn := aws.ToString(lb.LoadBalancerArn)
			name := aws.ToString(lb.LoadBalancerName)
			cr := &awsv1alpha1.LoadBalancer{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.LoadBalancerSpec{
					Name:   name,
					Type:   string(lb.Type),
					Scheme: string(lb.Scheme),
					Tags:   elbTagMap(ctx, clients, arn),
				},
			}
			for _, az := range lb.AvailabilityZones {
				subnetID := aws.ToString(az.SubnetId)
				if subnetID == "" {
					continue
				}
				if crName, ok := opts.Index.Lookup(subnetID); ok {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: subnetID})
				}
			}
			for _, sgID := range lb.SecurityGroups {
				if crName, ok := opts.Index.Lookup(sgID); ok {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
				} else {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
				}
			}
			// DeletionProtection and IdleTimeout live in attributes; failures
			// are non-fatal.
			attrs, err := clients.ELBv2.DescribeLoadBalancerAttributes(ctx, &awselbv2.DescribeLoadBalancerAttributesInput{
				LoadBalancerArn: lb.LoadBalancerArn,
			})
			if err == nil {
				for _, a := range attrs.Attributes {
					switch aws.ToString(a.Key) {
					case "deletion_protection.enabled":
						cr.Spec.DeletionProtection = aws.ToString(a.Value) == "true"
					case "idle_timeout.timeout_seconds":
						if n, err := strconv.Atoi(aws.ToString(a.Value)); err == nil {
							cr.Spec.IdleTimeout = int32(n)
						}
					}
				}
			}
			opts.Index.Add(arn, cr.Name)
			opts.Index.Add(aws.ToString(lb.DNSName), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// elbActions converts ELBv2 actions to spec actions. Actions the spec cannot
// represent (authenticate-*) are skipped.
func elbActions(actions []elbv2types.Action, opts *export.Options) []awsv1alpha1.ListenerDefaultAction {
	var out []awsv1alpha1.ListenerDefaultAction
	for _, a := range actions {
		act := awsv1alpha1.ListenerDefaultAction{Type: string(a.Type)}
		switch a.Type {
		case elbv2types.ActionTypeEnumForward:
			tgARN := aws.ToString(a.TargetGroupArn)
			if tgARN == "" && a.ForwardConfig != nil && len(a.ForwardConfig.TargetGroups) > 0 {
				tgARN = aws.ToString(a.ForwardConfig.TargetGroups[0].TargetGroupArn)
			}
			if tgARN == "" {
				continue
			}
			if crName, ok := opts.Index.Lookup(tgARN); ok {
				act.TargetGroupRef = &awsv1alpha1.TargetGroupRef{Name: crName}
			} else {
				act.TargetGroupRef = &awsv1alpha1.TargetGroupRef{ARN: tgARN}
			}
		case elbv2types.ActionTypeEnumRedirect:
			if a.RedirectConfig == nil {
				continue
			}
			act.RedirectConfig = &awsv1alpha1.ListenerRedirectConfig{
				StatusCode: string(a.RedirectConfig.StatusCode),
				Host:       aws.ToString(a.RedirectConfig.Host),
				Path:       aws.ToString(a.RedirectConfig.Path),
				Port:       aws.ToString(a.RedirectConfig.Port),
				Protocol:   aws.ToString(a.RedirectConfig.Protocol),
			}
		case elbv2types.ActionTypeEnumFixedResponse:
			if a.FixedResponseConfig == nil {
				continue
			}
			act.FixedResponseConfig = &awsv1alpha1.ListenerFixedResponseConfig{
				StatusCode:  aws.ToString(a.FixedResponseConfig.StatusCode),
				ContentType: aws.ToString(a.FixedResponseConfig.ContentType),
				MessageBody: aws.ToString(a.FixedResponseConfig.MessageBody),
			}
		default:
			// authenticate-cognito / authenticate-oidc are not in the spec.
			continue
		}
		out = append(out, act)
	}
	return out
}

func exportListeners(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	lbp := awselbv2.NewDescribeLoadBalancersPaginator(clients.ELBv2, &awselbv2.DescribeLoadBalancersInput{})
	for lbp.HasMorePages() {
		lbPage, err := lbp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range lbPage.LoadBalancers {
			lp := awselbv2.NewDescribeListenersPaginator(clients.ELBv2, &awselbv2.DescribeListenersInput{
				LoadBalancerArn: lb.LoadBalancerArn,
			})
			for lp.HasMorePages() {
				page, err := lp.NextPage(ctx)
				if err != nil {
					// Per-LB describe failure: skip this LB's listeners.
					break
				}
				for _, l := range page.Listeners {
					arn := aws.ToString(l.ListenerArn)
					name := fmt.Sprintf("%s-%s-%d",
						aws.ToString(lb.LoadBalancerName),
						strings.ToLower(string(l.Protocol)),
						aws.ToInt32(l.Port))
					cr := &awsv1alpha1.Listener{
						ObjectMeta: export.ObjectMeta(name, opts),
						Spec: awsv1alpha1.ListenerSpec{
							Protocol:       string(l.Protocol),
							Port:           aws.ToInt32(l.Port),
							DefaultActions: elbActions(l.DefaultActions, opts),
							SSLPolicy:      aws.ToString(l.SslPolicy),
							Tags:           elbTagMap(ctx, clients, arn),
						},
					}
					lbARN := aws.ToString(l.LoadBalancerArn)
					if crName, ok := opts.Index.Lookup(lbARN); ok {
						cr.Spec.LoadBalancerRef = awsv1alpha1.LoadBalancerRef{Name: crName}
					} else {
						cr.Spec.LoadBalancerRef = awsv1alpha1.LoadBalancerRef{ARN: lbARN}
					}
					for _, c := range l.Certificates {
						cr.Spec.CertificateARNs = append(cr.Spec.CertificateARNs, aws.ToString(c.CertificateArn))
					}
					opts.Index.Add(arn, cr.Name)
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportListenerRules(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	lbp := awselbv2.NewDescribeLoadBalancersPaginator(clients.ELBv2, &awselbv2.DescribeLoadBalancersInput{})
	for lbp.HasMorePages() {
		lbPage, err := lbp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range lbPage.LoadBalancers {
			lp := awselbv2.NewDescribeListenersPaginator(clients.ELBv2, &awselbv2.DescribeListenersInput{
				LoadBalancerArn: lb.LoadBalancerArn,
			})
			for lp.HasMorePages() {
				lPage, err := lp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, l := range lPage.Listeners {
					rp := awselbv2.NewDescribeRulesPaginator(clients.ELBv2, &awselbv2.DescribeRulesInput{
						ListenerArn: l.ListenerArn,
					})
					for rp.HasMorePages() {
						rPage, err := rp.NextPage(ctx)
						if err != nil {
							break
						}
						for _, rule := range rPage.Rules {
							// The default rule is the listener's own default
							// action, not a manageable rule.
							if aws.ToBool(rule.IsDefault) {
								continue
							}
							priority, err := strconv.Atoi(aws.ToString(rule.Priority))
							if err != nil {
								continue
							}
							name := fmt.Sprintf("%s-%s-%d-rule-%d",
								aws.ToString(lb.LoadBalancerName),
								strings.ToLower(string(l.Protocol)),
								aws.ToInt32(l.Port),
								priority)
							cr := &awsv1alpha1.ListenerRule{
								ObjectMeta: export.ObjectMeta(name, opts),
								Spec: awsv1alpha1.ListenerRuleSpec{
									Priority: int32(priority),
									Actions:  elbActions(rule.Actions, opts),
									Tags:     elbTagMap(ctx, clients, aws.ToString(rule.RuleArn)),
								},
							}
							listenerARN := aws.ToString(l.ListenerArn)
							if crName, ok := opts.Index.Lookup(listenerARN); ok {
								cr.Spec.ListenerRef = awsv1alpha1.ListenerRef{Name: crName}
							} else {
								cr.Spec.ListenerRef = awsv1alpha1.ListenerRef{ARN: listenerARN}
							}
							for _, c := range rule.Conditions {
								cr.Spec.Conditions = append(cr.Spec.Conditions, awsv1alpha1.RuleCondition{
									Field:  aws.ToString(c.Field),
									Values: c.Values,
								})
							}
							opts.Index.Add(aws.ToString(rule.RuleArn), cr.Name)
							objs = append(objs, cr)
						}
					}
				}
			}
		}
	}
	return objs, nil
}

func exportAutoScalingGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsautoscaling.NewDescribeAutoScalingGroupsPaginator(clients.AutoScaling, &awsautoscaling.DescribeAutoScalingGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe auto scaling groups: %w", err)
		}
		for _, asg := range page.AutoScalingGroups {
			name := aws.ToString(asg.AutoScalingGroupName)
			cr := &awsv1alpha1.AutoScalingGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.AutoScalingGroupSpec{
					AutoScalingGroupName: name,
					MinSize:              aws.ToInt32(asg.MinSize),
					MaxSize:              aws.ToInt32(asg.MaxSize),
					DesiredCapacity:      asg.DesiredCapacity,
					TargetGroupARNs:      asg.TargetGroupARNs,
				},
			}
			// LaunchTemplateRef is required by the spec; ASGs built on launch
			// configurations or mixed-instances policies cannot resolve it,
			// but the group is still exported with a direct ID when present.
			lt := asg.LaunchTemplate
			if lt == nil && asg.MixedInstancesPolicy != nil &&
				asg.MixedInstancesPolicy.LaunchTemplate != nil {
				lt = asg.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification
			}
			if lt != nil {
				ltID := aws.ToString(lt.LaunchTemplateId)
				if crName, ok := opts.Index.Lookup(ltID); ok {
					cr.Spec.LaunchTemplateRef = awsv1alpha1.LaunchTemplateRef{
						Name:    crName,
						Version: aws.ToString(lt.Version),
					}
				} else {
					cr.Spec.LaunchTemplateRef = awsv1alpha1.LaunchTemplateRef{
						ID:      ltID,
						Version: aws.ToString(lt.Version),
					}
				}
			}
			for _, subnetID := range strings.Split(aws.ToString(asg.VPCZoneIdentifier), ",") {
				subnetID = strings.TrimSpace(subnetID)
				if subnetID == "" {
					continue
				}
				if crName, ok := opts.Index.Lookup(subnetID); ok {
					cr.Spec.VPCZoneIdentifier = append(cr.Spec.VPCZoneIdentifier, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					cr.Spec.VPCZoneIdentifier = append(cr.Spec.VPCZoneIdentifier, awsv1alpha1.SubnetRef{ID: subnetID})
				}
			}
			if len(asg.Tags) > 0 {
				m := map[string]string{}
				for _, t := range asg.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(name, cr.Name)
			opts.Index.Add(aws.ToString(asg.AutoScalingGroupARN), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportScalingPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsautoscaling.NewDescribePoliciesPaginator(clients.AutoScaling, &awsautoscaling.DescribePoliciesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe scaling policies: %w", err)
		}
		for _, sp := range page.ScalingPolicies {
			policyName := aws.ToString(sp.PolicyName)
			asgName := aws.ToString(sp.AutoScalingGroupName)
			cr := &awsv1alpha1.ScalingPolicy{
				ObjectMeta: export.ObjectMeta(asgName+"-"+policyName, opts),
				Spec: awsv1alpha1.ScalingPolicySpec{
					PolicyName:        policyName,
					PolicyType:        aws.ToString(sp.PolicyType),
					AdjustmentType:    aws.ToString(sp.AdjustmentType),
					ScalingAdjustment: aws.ToInt32(sp.ScalingAdjustment),
					Cooldown:          aws.ToInt32(sp.Cooldown),
				},
			}
			// AutoScalingGroupRef is a plain ResourceRef (CR name only); fall
			// back to the sanitised AWS name when the ASG wasn't exported.
			if crName, ok := opts.Index.Lookup(asgName); ok {
				cr.Spec.AutoScalingGroupRef = awsv1alpha1.ResourceRef{Name: crName}
			} else {
				cr.Spec.AutoScalingGroupRef = awsv1alpha1.ResourceRef{Name: export.CRName(asgName)}
			}
			for _, sa := range sp.StepAdjustments {
				cr.Spec.StepAdjustments = append(cr.Spec.StepAdjustments, awsv1alpha1.StepAdjustment{
					ScalingAdjustment:        aws.ToInt32(sa.ScalingAdjustment),
					MetricIntervalLowerBound: sa.MetricIntervalLowerBound,
					MetricIntervalUpperBound: sa.MetricIntervalUpperBound,
				})
			}
			if ttc := sp.TargetTrackingConfiguration; ttc != nil {
				cr.Spec.TargetTrackingConfiguration = &awsv1alpha1.TargetTrackingConfiguration{
					TargetValue:    aws.ToFloat64(ttc.TargetValue),
					DisableScaleIn: aws.ToBool(ttc.DisableScaleIn),
				}
				if ttc.PredefinedMetricSpecification != nil {
					cr.Spec.TargetTrackingConfiguration.PredefinedMetricType = string(ttc.PredefinedMetricSpecification.PredefinedMetricType)
				}
			}
			opts.Index.Add(aws.ToString(sp.PolicyARN), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
