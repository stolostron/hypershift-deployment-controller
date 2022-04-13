package integration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	mcv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/controllers"
	// "github.com/stolostron/hypershift-deployment-controller/pkg/controllers/autoimport"
)

const (
	eventuallyTimeout  = 30 // seconds
	eventuallyInterval = 1  // seconds
)

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Integration Suite")
}

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

var (
	testEnv    *envtest.Environment
	restConfig *rest.Config
	ctx        context.Context
	cancel     context.CancelFunc
	mgr        ctrl.Manager
)

var _ = ginkgo.BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))
	ginkgo.By("bootstrapping test environment")

	var err error

	// install CRDs and start a local kube-apiserver
	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join(".", "..", "..", "config", "crd"),
			filepath.Join(".", "externalcrds"),
		},
	}
	cfg, err := testEnv.Start()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(cfg).ToNot(gomega.BeNil())
	restConfig = cfg

	ctx, cancel = context.WithCancel(context.Background())
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

var _ = ginkgo.AfterSuite(func() {
	if cancel != nil {
		cancel()
	}
})

func startCtrlManager(ctx context.Context, mgr ctrl.Manager) {
	err := (&controllers.HypershiftDeploymentReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		InfraHandler: &controllers.FakeInfraHandler{},
	}).SetupWithManager(mgr)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// err = (&autoimport.Reconciler{
	// 	Client: mgr.GetClient(),
	// 	Scheme: mgr.GetScheme(),
	// }).SetupWithManager(mgr)
	// gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = mgr.Start(ctrl.SetupSignalHandler())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}
