/*
Copyright 2022.

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

package controllers

import (
	"context"
	"errors"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hypershift/api/fixtures"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/infra/aws"
	"github.com/openshift/hypershift/cmd/infra/azure"
)

type InfraHandler interface {
	AwsInfraCreator(awsKey, awsSecretKey, region, infraID, name, baseDomain string, zones []string) AwsCreateInfra
	AwsInfraDestroyer(awsKey, awsSecretKey, region, infraID, name, baseDomain string) AwsDestroyInfra
	AwsIAMCreator(awsKey, awsSecretKey, region, infraID, s3BucketName, s3Region, privateZoneID, publicZoneID, localZoneID string) AwsCreateIAM
	AwsIAMDestroyer(awsKey, awsSecretKey, region, infraID string) AwsDestroyIAM

	AzureInfraDestroyer(name, location, infraID string, credentials *fixtures.AzureCreds) AzureDestroyInfra
	AzureInfraCreator(name, baseDomain, location, infraID string, credentials *fixtures.AzureCreds) AzureCreateInfra
}

type AwsCreateInfra func(ctx context.Context) (*aws.CreateInfraOutput, error)
type AwsDestroyInfra func(ctx context.Context) error
type AwsCreateIAM func(ctx context.Context, client crclient.Client) (*aws.CreateIAMOutput, error)
type AwsDestroyIAM func(ctx context.Context) error
type AzureDestroyInfra func(ctx context.Context) error
type AzureCreateInfra func(ctx context.Context) (*azure.CreateInfraOutput, error)

var _ InfraHandler = &DefaultInfraHandler{}

type DefaultInfraHandler struct{}

func (h *DefaultInfraHandler) AwsInfraCreator(awsKey, awsSecretKey, region, infraID, name, baseDomain string, zones []string) AwsCreateInfra {
	o := &aws.CreateInfraOptions{
		AWSKey:       awsKey,
		AWSSecretKey: awsSecretKey,
		Region:       region,
		Zones:        zones,
		InfraID:      infraID,
		Name:         name,
		BaseDomain:   baseDomain,
	}
	return o.CreateInfra
}

func (h *DefaultInfraHandler) AwsInfraDestroyer(awsKey, awsSecretKey, region, infraID, name, baseDomain string) AwsDestroyInfra {
	o := &aws.DestroyInfraOptions{
		AWSKey:       awsKey,
		AWSSecretKey: awsSecretKey,
		Region:       region,
		InfraID:      infraID,
		Name:         name,
		BaseDomain:   baseDomain,
	}
	return o.DestroyInfra
}

func (h *DefaultInfraHandler) AwsIAMCreator(awsKey, awsSecretKey, region, infraID, s3BucketName, s3Region, privateZoneID, publicZoneID, localZoneID string) AwsCreateIAM {
	iamOpt := aws.CreateIAMOptions{
		Region:       region,
		AWSKey:       awsKey,
		AWSSecretKey: awsSecretKey,
		InfraID:      infraID,
		// IssuerURL:                       "", //This is generated on the fly by CreateIAMOutput
		// AdditionalTags:                  []string{},
		OIDCStorageProviderS3BucketName: s3BucketName,
		OIDCStorageProviderS3Region:     s3Region,
		PrivateZoneID:                   privateZoneID,
		PublicZoneID:                    publicZoneID,
		LocalZoneID:                     localZoneID,
	}

	return iamOpt.CreateIAM
}

func (h *DefaultInfraHandler) AwsIAMDestroyer(awsKey, awsSecretKey, region, infraID string) AwsDestroyIAM {
	iamOpt := aws.DestroyIAMOptions{
		Region:       region,
		AWSKey:       awsKey,
		AWSSecretKey: awsSecretKey,
		InfraID:      infraID,
	}

	return iamOpt.DestroyIAM
}

func (h *DefaultInfraHandler) AzureInfraDestroyer(name, location, infraID string, credentials *fixtures.AzureCreds) AzureDestroyInfra {
	dOpts := azure.DestroyInfraOptions{
		Location:    location,
		Credentials: credentials,
		Name:        name,
		InfraID:     infraID,
	}

	return dOpts.Run
}

func (h *DefaultInfraHandler) AzureInfraCreator(name, baseDomain, location, infraID string, credentials *fixtures.AzureCreds) AzureCreateInfra {
	o := azure.CreateInfraOptions{
		Location:    location,
		InfraID:     infraID,
		Name:        name,
		BaseDomain:  baseDomain,
		Credentials: credentials,
	}
	return o.Run
}

var _ InfraHandler = &FakeInfraHandler{}

type FakeInfraHandler struct{}

type FakeInfraHandlerFailure struct{}

func (h *FakeInfraHandler) AwsInfraCreator(awsKey, awsSecretKey, region, infraID, name, baseDomain string, zones []string) AwsCreateInfra {
	return func(ctx context.Context) (*aws.CreateInfraOutput, error) {
		return &aws.CreateInfraOutput{
			Zones: []*aws.CreateInfraOutputZone{
				{
					Name:     "us-east-1a",
					SubnetID: "subnet-0123456789abcdefg",
				},
				{
					Name:     "us-east-1b",
					SubnetID: "subnet-00000011111222233",
				},
			},
			VPCID:           "vpc-abcdefg0123456789",
			PrivateZoneID:   "ABCDEFGHIJKLMNOPQRSTU",
			PublicZoneID:    "ABCDEFGHIJKLMN",
			BaseDomain:      "a.b.c",
			ComputeCIDR:     "127.0.0.0/16",
			SecurityGroupID: "sg-a1b2c3d4e5f6g7hig",
			LocalZoneID:     "ABCDEFGHIJKLMN123456",
		}, nil
	}
}

func (h *FakeInfraHandlerFailure) AwsInfraCreator(awsKey, awsSecretKey, region, infraID, name, baseDomain string, zones []string) AwsCreateInfra {
	return func(ctx context.Context) (*aws.CreateInfraOutput, error) {
		return nil, errors.New("failed to create aws infrastructure")
	}
}

func (h *FakeInfraHandler) AwsInfraDestroyer(awsKey, awsSecretKey, region, infraID, name, baseDomain string) AwsDestroyInfra {
	return func(ctx context.Context) error {
		return nil
	}
}

func (h *FakeInfraHandlerFailure) AwsInfraDestroyer(awsKey, awsSecretKey, region, infraID, name, baseDomain string) AwsDestroyInfra {
	return func(ctx context.Context) error {
		return errors.New("failed to destroy aws infrastructure")
	}
}

func (h *FakeInfraHandler) AwsIAMCreator(awsKey, awsSecretKey, region, infraID, s3BucketName, s3Region, privateZoneID, publicZoneID, localZoneID string) AwsCreateIAM {
	return func(ctx context.Context, client crclient.Client) (*aws.CreateIAMOutput, error) {
		return &aws.CreateIAMOutput{
			ControlPlaneOperatorRoleARN: "arn:aws:iam::012345678910:role/hypershift-test-abcde-control-plane-operator",
			KubeCloudControllerRoleARN:  "arn:aws:iam::012345678910:role/hypershift-test-abcde-cloud-controller",
			NodePoolManagementRoleARN:   "arn:aws:iam::012345678910:role/hypershift-test-abcde-node-pool",
			IssuerURL:                   "https://bucket-hypershift.s3.us-east-1.amazonaws.com/hypershift-test-abcde",
			Roles: []hyperv1.AWSRoleCredentials{
				{
					ARN:       "arn:aws:iam::012345678910:role/hypershift-test-abcde-openshift-ingress",
					Name:      "cloud-credentials",
					Namespace: "openshift-ingress-operator",
				},
				{
					ARN:       "arn:aws:iam::012345678910:role/hypershift-test-abcde-openshift-image-registry",
					Name:      "installer-cloud-credentials",
					Namespace: "openshift-image-registry",
				},
				{
					ARN:       "arn:aws:iam::012345678910:role/hypershift-test-abcde-aws-ebs-csi-driver-controller",
					Name:      "ebs-cloud-credentials",
					Namespace: "openshift-cluster-csi-drivers",
				},
				{
					ARN:       "arn:aws:iam::012345678910:role/hypershift-test-abcde-cloud-network-config-controller",
					Name:      "cloud-credentials",
					Namespace: "openshift-cloud-network-config-controller",
				},
			},
		}, nil
	}
}

func (h *FakeInfraHandlerFailure) AwsIAMCreator(awsKey, awsSecretKey, region, infraID, s3BucketName, s3Region, privateZoneID, publicZoneID, localZoneID string) AwsCreateIAM {
	return func(ctx context.Context, client crclient.Client) (*aws.CreateIAMOutput, error) {
		return nil, errors.New("failed to create aws iam infrastructure")
	}
}

func (h *FakeInfraHandler) AwsIAMDestroyer(awsKey, awsSecretKey, region, infraID string) AwsDestroyIAM {
	return func(ctx context.Context) error {
		return nil
	}
}

func (h *FakeInfraHandlerFailure) AwsIAMDestroyer(awsKey, awsSecretKey, region, infraID string) AwsDestroyIAM {
	return func(ctx context.Context) error {
		return errors.New("failed to destroy aws iam infrastructure")
	}
}

func (h *FakeInfraHandler) AzureInfraDestroyer(name, location, infraID string, credentials *fixtures.AzureCreds) AzureDestroyInfra {
	return func(ctx context.Context) error {
		return nil
	}
}

func (h *FakeInfraHandlerFailure) AzureInfraDestroyer(name, location, infraID string, credentials *fixtures.AzureCreds) AzureDestroyInfra {
	return func(ctx context.Context) error {
		return errors.New("failed to destroy azure infrastructure")
	}
}

func (h *FakeInfraHandler) AzureInfraCreator(name, baseDomain, location, infraID string, credentials *fixtures.AzureCreds) AzureCreateInfra {
	return func(ctx context.Context) (*azure.CreateInfraOutput, error) {
		return &azure.CreateInfraOutput{
			Location:          "centralus",
			MachineIdentityID: "/subscriptions/abcd1234-5678-123a-ab1c-asdfgh098765/resourcegroups/hypershift-test-hypershift-test-abcde/providers/Microsoft.ManagedIdentity/userAssignedIdentities/hypershift-test-hypershift-test-abcde",
			ResourceGroupName: "hypershift-test-hypershift-test-abcde",
			SecurityGroupName: "hypershift-test-hypershift-test-abcde-abc",
			SubnetName:        "default",
			VNetID:            "/subscriptions/abcd1234-5678-123a-ab1c-asdfgh098765/resourceGroups/hypershift-test-hypershift-test-abcde/providers/Microsoft.Network/virtualNetworks/hypershift-test-hypershift-test-abcde",
			VnetName:          "hypershift-test-hypershift-test-abcde",
			BaseDomain:        "a.b.c",
			PublicZoneID:      "/subscriptions/abcd1234-5678-123a-ab1c-asdfgh098765/resourceGroups/os4-common/providers/Microsoft.Network/dnszones/a.b.c",
			PrivateZoneID:     "/subscriptions/abcd1234-5678-123a-ab1c-asdfgh098765/resourceGroups/hypershift-test-hypershift-test-abcde/providers/Microsoft.Network/privateDnsZones/hypershift-test-azurecluster.a.b.c",
			BootImageID:       "/subscriptions/abcd1234-5678-123a-ab1c-asdfgh098765/resourceGroups/hypershift-test-hypershift-test-abcde/providers/Microsoft.Compute/images/rhcos.x86_64.vhd",
		}, nil
	}
}

func (h *FakeInfraHandlerFailure) AzureInfraCreator(name, baseDomain, location, infraID string, credentials *fixtures.AzureCreds) AzureCreateInfra {
	return func(ctx context.Context) (*azure.CreateInfraOutput, error) {
		return nil, errors.New("failed to create azure infrastructure")
	}
}
