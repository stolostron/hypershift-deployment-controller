package integration_test

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	mcv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/controllers"
	"github.com/stolostron/hypershift-deployment-controller/pkg/controllers/autoimport"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(hypdeployment.AddToScheme(scheme))
	utilruntime.Must(hyp.AddToScheme(scheme))
	utilruntime.Must(workv1.AddToScheme(scheme))
	utilruntime.Must(mcv1.AddToScheme(scheme))
}

func startCtrlManager(ctx context.Context, mgr ctrl.Manager) {
	err := (&controllers.HypershiftDeploymentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = (&autoimport.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = mgr.Start(ctrl.SetupSignalHandler())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

var _ = ginkgo.Describe("Manifest Work", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		mgr    ctrl.Manager
	)

	ginkgo.BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		var err error
		mgr, err = ctrl.NewManager(restConfig, ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     ":8080",
			Port:                   9443,
			HealthProbeBindAddress: ":8081",
			LeaderElection:         false,
			LeaderElectionID:       "dfe33d84.open-cluster-management.io",
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		go startCtrlManager(ctx, mgr)
	})

	ginkgo.AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	ginkgo.Context("test infra config false", func() {
		var (
			hydName      string
			hydNamespace string
			hyd          *hypdeployment.HypershiftDeployment
			infraID      string
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
		})
		ginkgo.AfterEach(func() {
			err := mgr.GetClient().Delete(ctx, hyd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("aws", func() {
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
				manifestworks := workv1.ManifestWorkList{}
				err = mgr.GetClient().List(ctx, &manifestworks)
				if err != nil {
					return false
				}
				if len(manifestworks.Items) > 0 {
					return true
				}
				return false
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
		})
	})
})
