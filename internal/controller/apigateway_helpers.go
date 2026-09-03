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

import (
	"context"
	"fmt"
	"strings"
	"time"

	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// requeueAPIGWPolling is the poll interval for async API Gateway resources
// (VPC links transition PENDING -> AVAILABLE).
var requeueAPIGWPolling = ctrl.Result{RequeueAfter: 15 * time.Second}

// resolveAPIGatewayV2APIID resolves an APIRef against APIGatewayV2API CRs to
// an AWS API ID. Uses direct .APIID if set, otherwise looks up the CR's
// status.apiId, returning dependencyNotReady while it is unset.
func resolveAPIGatewayV2APIID(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.APIRef) (string, error) {
	if ref.APIID != "" {
		return ref.APIID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("apiRef requires name or apiId")
	}
	apiCR := &awsv1alpha1.APIGatewayV2API{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, apiCR); err != nil {
		return "", err
	}
	if apiCR.Status.APIID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("APIGatewayV2API %s/%s has no apiId yet", namespace, ref.Name)}
	}
	return apiCR.Status.APIID, nil
}

// resolveRestAPIID resolves an APIRef against RestAPI CRs to an AWS REST API
// ID. Uses direct .APIID if set, otherwise looks up the CR's status.apiId,
// returning dependencyNotReady while it is unset.
func resolveRestAPIID(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.APIRef) (string, error) {
	if ref.APIID != "" {
		return ref.APIID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("restApiRef requires name or apiId")
	}
	apiCR := &awsv1alpha1.RestAPI{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, apiCR); err != nil {
		return "", err
	}
	if apiCR.Status.APIID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("RestAPI %s/%s has no apiId yet", namespace, ref.Name)}
	}
	return apiCR.Status.APIID, nil
}

// lambdaInvocationURI converts a Lambda function ARN into the API Gateway
// Lambda invocation URI required for REQUEST authorizers. Non-ARN values are
// returned unchanged (assumed to already be an invocation URI).
func lambdaInvocationURI(fnARN string) string {
	if !strings.HasPrefix(fnARN, "arn:") {
		return fnARN
	}
	parts := strings.Split(fnARN, ":")
	if len(parts) < 4 {
		return fnARN
	}
	return fmt.Sprintf("arn:aws:apigateway:%s:lambda:path/2015-03-31/functions/%s/invocations", parts[3], fnARN)
}
