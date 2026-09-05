/*
Copyright 2024.

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

// Package eks provides helper functions for EKS Pod Identity operations.
package eks

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

// IsNotFound returns true when the error indicates the resource does not exist.
func IsNotFound(err error) bool {
	var rnf *types.ResourceNotFoundException
	return errors.As(err, &rnf)
}

// PodIdentityAPI is the narrow subset of the EKS SDK client used by the Pod
// Identity association helpers in this file. *multi.EKS satisfies it.
type PodIdentityAPI interface {
	ListPodIdentityAssociations(ctx context.Context, params *eks.ListPodIdentityAssociationsInput, optFns ...func(*eks.Options)) (*eks.ListPodIdentityAssociationsOutput, error)
	DescribePodIdentityAssociation(ctx context.Context, params *eks.DescribePodIdentityAssociationInput, optFns ...func(*eks.Options)) (*eks.DescribePodIdentityAssociationOutput, error)
	CreatePodIdentityAssociation(ctx context.Context, params *eks.CreatePodIdentityAssociationInput, optFns ...func(*eks.Options)) (*eks.CreatePodIdentityAssociationOutput, error)
	UpdatePodIdentityAssociation(ctx context.Context, params *eks.UpdatePodIdentityAssociationInput, optFns ...func(*eks.Options)) (*eks.UpdatePodIdentityAssociationOutput, error)
	DeletePodIdentityAssociation(ctx context.Context, params *eks.DeletePodIdentityAssociationInput, optFns ...func(*eks.Options)) (*eks.DeletePodIdentityAssociationOutput, error)
}

// FindAssociation looks up an existing Pod Identity association by cluster,
// namespace, and service account name. Returns nil, nil if not found.
func FindAssociation(
	ctx context.Context,
	client PodIdentityAPI,
	clusterName, namespace, serviceAccountName string,
) (*types.PodIdentityAssociationSummary, error) {
	paginator := eks.NewListPodIdentityAssociationsPaginator(client, &eks.ListPodIdentityAssociationsInput{
		ClusterName:    aws.String(clusterName),
		Namespace:      aws.String(namespace),
		ServiceAccount: aws.String(serviceAccountName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for i := range page.Associations {
			a := &page.Associations[i]
			if aws.ToString(a.Namespace) == namespace &&
				aws.ToString(a.ServiceAccount) == serviceAccountName {
				return a, nil
			}
		}
	}
	return nil, nil
}

// GetAssociation fetches full details of a Pod Identity association by ID.
func GetAssociation(ctx context.Context, client PodIdentityAPI, clusterName, associationID string) (*types.PodIdentityAssociation, error) {
	out, err := client.DescribePodIdentityAssociation(ctx, &eks.DescribePodIdentityAssociationInput{
		ClusterName:   aws.String(clusterName),
		AssociationId: aws.String(associationID),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.Association, nil
}

// CreateAssociation creates an EKS Pod Identity association.
func CreateAssociation(
	ctx context.Context,
	client PodIdentityAPI,
	clusterName, namespace, serviceAccount, roleARN string,
	tags map[string]string,
) (*types.PodIdentityAssociation, error) {
	out, err := client.CreatePodIdentityAssociation(ctx, &eks.CreatePodIdentityAssociationInput{
		ClusterName:    aws.String(clusterName),
		Namespace:      aws.String(namespace),
		ServiceAccount: aws.String(serviceAccount),
		RoleArn:        aws.String(roleARN),
		Tags:           tags,
	})
	if err != nil {
		return nil, err
	}
	return out.Association, nil
}

// UpdateAssociation updates the role ARN on an existing Pod Identity association.
func UpdateAssociation(ctx context.Context, client PodIdentityAPI, clusterName, associationID, roleARN string) (*types.PodIdentityAssociation, error) {
	out, err := client.UpdatePodIdentityAssociation(ctx, &eks.UpdatePodIdentityAssociationInput{
		ClusterName:   aws.String(clusterName),
		AssociationId: aws.String(associationID),
		RoleArn:       aws.String(roleARN),
	})
	if err != nil {
		return nil, err
	}
	return out.Association, nil
}

// DeleteAssociation deletes an EKS Pod Identity association. Tolerates not-found.
func DeleteAssociation(ctx context.Context, client PodIdentityAPI, clusterName, associationID string) error {
	_, err := client.DeletePodIdentityAssociation(ctx, &eks.DeletePodIdentityAssociationInput{
		ClusterName:   aws.String(clusterName),
		AssociationId: aws.String(associationID),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}
