package integration_test

import (
	"encoding/json"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hypershift/api/fixtures"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
)

var _ = ginkgo.Describe("Manifest Work", func() {
	var (
		hydName        string
		hydNamespace   string
		hyd            *hypdeployment.HypershiftDeployment
		infraID        string
		s3bucketSecret *corev1.Secret
	)
	ginkgo.BeforeEach(func() {
		hydName = fmt.Sprintf("hyd-%s", rand.String(6))
		hydNamespace = "default"
		infraID = fmt.Sprintf("%s-%s", hydName, rand.String(5))
		hyd = &hypdeployment.HypershiftDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hydName,
				Namespace: hydNamespace,
			}}

		s3bucketSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hypershift-operator-oidc-provider-s3-credentials",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"bucket":      []byte("test-bucket"),
				"credentials": []byte("test-credentials"),
			},
		}
		err := mgr.GetClient().Create(ctx, s3bucketSecret)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
	ginkgo.AfterEach(func() {
		err := mgr.GetClient().Delete(ctx, s3bucketSecret)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
	ginkgo.Context("aws", func() {
		var cloudProviderSecret *corev1.Secret
		ginkgo.BeforeEach(func() {
			hydName = fmt.Sprintf("hyd-%s", rand.String(6))
			hydNamespace = "default"
			infraID = fmt.Sprintf("%s-%s", hydName, rand.String(5))
			hyd = &hypdeployment.HypershiftDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hydName,
					Namespace: hydNamespace,
				}}

			cloudProviderSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-aws",
					Namespace: hydNamespace,
				},
				Data: map[string][]byte{
					"aws_access_key_id":     []byte("key-id"),
					"aws_secret_access_key": []byte("access-key"),
					"baseDomain":            []byte("a.b.c"),
					"pullSecret":            []byte("test-pull-secret"),
				},
			}
			err := mgr.GetClient().Create(ctx, cloudProviderSecret)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
		ginkgo.AfterEach(func() {
			hydpreview := hypdeployment.HypershiftDeployment{}
			err := mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hydNamespace, Name: hydName}, &hydpreview)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.By(fmt.Sprintf("\nname: %s, ##### conditions #####:\n %v\n\n", hydName, hydpreview.Status.Conditions))

			err = mgr.GetClient().Delete(ctx, hyd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() bool {
				hydDeleting := hypdeployment.HypershiftDeployment{}
				err := mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hydNamespace, Name: hydName}, &hydDeleting)
				return apierrors.IsNotFound(err)
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			err = mgr.GetClient().Delete(ctx, cloudProviderSecret)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("infra config false without pull secret", func() {
			hyd.Spec = hypdeployment.HypershiftDeploymentSpec{
				InfraID:          infraID,
				Override:         hypdeployment.DeleteHostingNamespace,
				HostingNamespace: "clusters",
				HostingCluster:   "default",
				Infrastructure: hypdeployment.InfraSpec{
					Configure: false,
				},
				HostedClusterSpec: &hyp.HostedClusterSpec{
					Platform: hyp.PlatformSpec{
						Type: hyp.AWSPlatform,
					},
					Networking: hyp.ClusterNetworking{
						NetworkType: hyp.OpenShiftSDN,
					},
					Services: []hyp.ServicePublishingStrategyMapping{},
					Release: hyp.Release{
						Image: constant.ReleaseImage,
					},
					Etcd: hyp.EtcdSpec{
						ManagementType: hyp.Managed,
					},
				},
			}

			err := mgr.GetClient().Create(ctx, hyd, &client.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				manifestwork := workv1.ManifestWork{}
				err = mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hyd.Spec.HostingCluster, Name: infraID}, &manifestwork)
				if err != nil {
					return false
				}

				// HostedCluster + NodePool
				if len(manifestwork.Spec.Workload.Manifests) != 2 {
					return false
				}

				return true
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("infra config false with pull secret", func() {
			hyd.Spec = hypdeployment.HypershiftDeploymentSpec{
				InfraID:          infraID,
				Override:         hypdeployment.DeleteHostingNamespace,
				HostingNamespace: "clusters",
				HostingCluster:   "default",
				Infrastructure: hypdeployment.InfraSpec{
					Configure: false,
				},
				HostedClusterSpec: &hyp.HostedClusterSpec{
					Platform: hyp.PlatformSpec{
						Type: hyp.AWSPlatform,
					},
					Networking: hyp.ClusterNetworking{
						NetworkType: hyp.OpenShiftSDN,
					},
					Services: []hyp.ServicePublishingStrategyMapping{},
					Release: hyp.Release{
						Image: constant.ReleaseImage,
					},
					Etcd: hyp.EtcdSpec{
						ManagementType: hyp.Managed,
					},
					PullSecret: corev1.LocalObjectReference{
						Name: cloudProviderSecret.Name,
					},
				},
			}

			err := mgr.GetClient().Create(ctx, hyd, &client.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				manifestwork := workv1.ManifestWork{}
				err = mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hyd.Spec.HostingCluster, Name: infraID}, &manifestwork)
				if err != nil {
					return false
				}

				// HostedCluster + NodePool + pullSecret
				if len(manifestwork.Spec.Workload.Manifests) != 3 {
					return false
				}

				return true
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("infra config true", func() {
			hyd.Spec = hypdeployment.HypershiftDeploymentSpec{
				InfraID:          infraID,
				Override:         hypdeployment.DeleteHostingNamespace,
				HostingNamespace: "clusters",
				HostingCluster:   "default",
				Infrastructure: hypdeployment.InfraSpec{
					Configure: true,
					Platform: &hypdeployment.Platforms{
						AWS: &hypdeployment.AWSPlatform{
							Region: "us-east-1",
						},
					},
					CloudProvider: corev1.LocalObjectReference{
						Name: cloudProviderSecret.Name,
					},
				},
			}

			err := mgr.GetClient().Create(ctx, hyd, &client.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				manifestwork := workv1.ManifestWork{}
				err = mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hyd.Spec.HostingCluster, Name: infraID}, &manifestwork)
				if err != nil {
					return false
				}

				// Namespace + HostedCluster + NodePool + pullSecret + 3 awsArnSecrets + etcd encryption secret
				if len(manifestwork.Spec.Workload.Manifests) != 8 {
					return false
				}

				// TODO: verify the contents of the manifests.
				return true
			}, 30, eventuallyInterval).Should(gomega.BeTrue())
		})
	})

	ginkgo.Context("azure", func() {
		var cloudProviderSecret *corev1.Secret
		ginkgo.BeforeEach(func() {
			hydName = fmt.Sprintf("hyd-%s", rand.String(6))
			hydNamespace = "default"
			infraID = fmt.Sprintf("%s-%s", hydName, rand.String(5))
			hyd = &hypdeployment.HypershiftDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hydName,
					Namespace: hydNamespace,
				}}

			credentials := &fixtures.AzureCreds{
				SubscriptionID: "abcd1234-5678-123a-ab1c-asdfgh098765",
				TenantID:       "qazwsx12-1234-5678-9100-qazwsxedc123",
				ClientID:       "asdfg987-qwer-1234-asdf-mnbvcx123456",
				ClientSecret:   "test-foobar",
			}
			credentialsBytes, err := json.Marshal(credentials)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			cloudProviderSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-azure",
					Namespace: hydNamespace,
				},
				Data: map[string][]byte{
					"baseDomain":              []byte("a.b.c"),
					"pullSecret":              []byte("test-pull-secret"),
					"osServicePrincipal.json": credentialsBytes,
				},
			}
			err = mgr.GetClient().Create(ctx, cloudProviderSecret)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
		ginkgo.AfterEach(func() {
			hydpreview := hypdeployment.HypershiftDeployment{}
			err := mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hydNamespace, Name: hydName}, &hydpreview)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.By(fmt.Sprintf("\nname: %s, ##### conditions #####:\n %v\n\n", hydName, hydpreview.Status.Conditions))

			err = mgr.GetClient().Delete(ctx, hyd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() bool {
				hydDeleting := hypdeployment.HypershiftDeployment{}
				err := mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hydNamespace, Name: hydName}, &hydDeleting)
				return apierrors.IsNotFound(err)
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			err = mgr.GetClient().Delete(ctx, cloudProviderSecret)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("infra config false without pull secret", func() {
			hyd.Spec = hypdeployment.HypershiftDeploymentSpec{
				InfraID:          infraID,
				Override:         hypdeployment.DeleteHostingNamespace,
				HostingNamespace: "clusters",
				HostingCluster:   "default",
				Infrastructure: hypdeployment.InfraSpec{
					Configure: false,
				},
				HostedClusterSpec: &hyp.HostedClusterSpec{
					Platform: hyp.PlatformSpec{
						Type: hyp.AzurePlatform,
					},
					Networking: hyp.ClusterNetworking{
						NetworkType: hyp.OpenShiftSDN,
					},
					Services: []hyp.ServicePublishingStrategyMapping{},
					Release: hyp.Release{
						Image: constant.ReleaseImage,
					},
					Etcd: hyp.EtcdSpec{
						ManagementType: hyp.Managed,
					},
				},
			}

			err := mgr.GetClient().Create(ctx, hyd, &client.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				manifestwork := workv1.ManifestWork{}
				err = mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hyd.Spec.HostingCluster, Name: infraID}, &manifestwork)
				if err != nil {
					return false
				}

				// HostedCluster + NodePool
				if len(manifestwork.Spec.Workload.Manifests) != 2 {
					return false
				}

				return true
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("infra config false with pull secret", func() {
			hyd.Spec = hypdeployment.HypershiftDeploymentSpec{
				InfraID:          infraID,
				Override:         hypdeployment.DeleteHostingNamespace,
				HostingNamespace: "clusters",
				HostingCluster:   "default",
				Infrastructure: hypdeployment.InfraSpec{
					Configure: false,
				},
				HostedClusterSpec: &hyp.HostedClusterSpec{
					Platform: hyp.PlatformSpec{
						Type: hyp.AzurePlatform,
					},
					Networking: hyp.ClusterNetworking{
						NetworkType: hyp.OpenShiftSDN,
					},
					Services: []hyp.ServicePublishingStrategyMapping{},
					Release: hyp.Release{
						Image: constant.ReleaseImage,
					},
					Etcd: hyp.EtcdSpec{
						ManagementType: hyp.Managed,
					},
					PullSecret: corev1.LocalObjectReference{
						Name: cloudProviderSecret.Name,
					},
				},
			}

			err := mgr.GetClient().Create(ctx, hyd, &client.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				manifestwork := workv1.ManifestWork{}
				err = mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hyd.Spec.HostingCluster, Name: infraID}, &manifestwork)
				if err != nil {
					return false
				}

				// HostedCluster + NodePool + pullSecret
				if len(manifestwork.Spec.Workload.Manifests) != 3 {
					return false
				}

				return true
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("infra config true", func() {
			hyd.Spec = hypdeployment.HypershiftDeploymentSpec{
				InfraID:          infraID,
				Override:         hypdeployment.DeleteHostingNamespace,
				HostingNamespace: "clusters",
				HostingCluster:   "default",
				Infrastructure: hypdeployment.InfraSpec{
					Configure: true,
					Platform: &hypdeployment.Platforms{
						Azure: &hypdeployment.AzurePlatform{
							Location: "eastus",
						},
					},
					CloudProvider: corev1.LocalObjectReference{
						Name: cloudProviderSecret.Name,
					},
				},
			}

			err := mgr.GetClient().Create(ctx, hyd, &client.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				manifestwork := workv1.ManifestWork{}
				err = mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: hyd.Spec.HostingCluster, Name: infraID}, &manifestwork)
				if err != nil {
					return false
				}

				// Namespace + HostedCluster + NodePool + pullSecret + 1 azureCloudCredential + etcd encryption secret
				if len(manifestwork.Spec.Workload.Manifests) != 6 {
					return false
				}

				// TODO: verify the contents of the manifests.
				return true
			}, 30, eventuallyInterval).Should(gomega.BeTrue())
		})
	})
})
