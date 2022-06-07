package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	apifixtures "github.com/openshift/hypershift/api/fixtures"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	hyd "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getHDforSecretEncryption(config bool) *hyd.HypershiftDeployment {
	return &hyd.HypershiftDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				"test1": "doNotTransfer1",
				"test2": "doNotTransfer2",
			},
		},
		Spec: hyd.HypershiftDeploymentSpec{
			HostingCluster:   "local-cluster",
			HostingNamespace: "clusters",
			Infrastructure: hyd.InfraSpec{
				Configure: config,
				Platform: &hyd.Platforms{
					AWS: &hyd.AWSPlatform{Region: "us-east-1"},
				},
			},
			InfraID: "test1-abcde",
		},
	}
}

func getHostedCluster(hyd *hyd.HypershiftDeployment) *hyp.HostedCluster {
	hc := &hyp.HostedCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HostedCluster",
			APIVersion: "hypershift.openshift.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testHostedCluster",
			Namespace: "default",
		},
		Spec: hyp.HostedClusterSpec{
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
	hyd.Spec.HostedClusterRef = corev1.LocalObjectReference{Name: hc.Name}
	return hc
}

func getNodePools(hyd *hyd.HypershiftDeployment) []*hyp.NodePool {
	hyd.Spec.NodePoolsRef = []corev1.LocalObjectReference{{Name: "testNodePool"}}
	return []*hyp.NodePool{{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: "hypershift.openshift.io/v1alpha1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "testNodePool",
			Namespace: "default",
		},
		Spec: hyp.NodePoolSpec{
			ClusterName: "testHostedCluster",
			Platform: hyp.NodePoolPlatform{
				Type: hyp.AWSPlatform,
			},
			Release: hyp.Release{
				Image: constant.ReleaseImage,
			},
		},
	}}
}

// TestHDEncryptionSecret tests if the manifestwork is created
// with the encryption secret
func TestHDEncryptionSecret(t *testing.T) {
	r := GetHypershiftDeploymentReconciler()
	ctx := context.Background()

	testHD := getHypershiftDeployment("default", "test1", false)
	hostedCluster := getHostedCluster(testHD)
	initFakeClient(r, hostedCluster)

	// Create AESCBC active key secret
	exampleOptions := &apifixtures.ExampleOptions{
		Name:      "test-my",
		Namespace: "default",
	}
	userActiveKeySecret := exampleOptions.EtcdEncryptionKeySecret()
	err := r.Create(ctx, userActiveKeySecret)
	defer r.Delete(ctx, userActiveKeySecret)
	assert.Nil(t, err, "active encryption secret should be created with no error")

	// Create AESCBC backup key secret
	userBackupKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-backup-key",
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}
	err = r.Create(ctx, userBackupKeySecret)
	defer r.Delete(ctx, userBackupKeySecret)
	assert.Nil(t, err, "backup encryption secret should be created with no error")

	// Test configure=T - no encryption secret specified - generate AESCBC encryption secret
	configTHD := getHDforSecretEncryption(true)
	scaffoldHostedClusterSpec(configTHD)
	assert.Equal(t, hyp.AESCBC, configTHD.Spec.HostedClusterSpec.SecretEncryption.Type, "secretEncryption should default to AESCBC for configure=T")
	assert.Equal(t, configTHD.Name+"-etcd-encryption-key", configTHD.Spec.HostedClusterSpec.SecretEncryption.AESCBC.ActiveKey.Name, "AESCBC active key is not set correctly for secret encryption")

	m, err := scaffoldManifestwork(configTHD)
	assert.Nil(t, err)

	payload := []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest := r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 2, "2 manifestwork payload which is the hostedcluster and the generated encryption secret")
	payloadSec, _ := getManifestPayloadSecretByName(&payload, configTHD.Name+"-etcd-encryption-key")
	assert.NotNil(t, payloadSec, "is not nil when encryption secret is found")
	assert.Equal(t, configTHD.Spec.HostingNamespace, payloadSec.GetNamespace())

	// Test configure=T - use encryption secret found in old manifestwork payload
	oSec := payloadSec
	oSecPayload := workv1.Manifest{}
	oSecPayload.Raw, _ = json.Marshal(oSec)
	m.Spec.Workload.Manifests = []workv1.Manifest{oSecPayload}

	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 2, "2 manifestwork payload which is the hostedcluster and the generated encryption secret")
	payloadSec, _ = getManifestPayloadSecretByName(&payload, configTHD.Name+"-etcd-encryption-key")
	assert.NotNil(t, payloadSec, "is not nil when encryption secret is found")
	assert.Equal(t, configTHD.Spec.HostingNamespace, payloadSec.GetNamespace())
	// encryption secret is not re-generated, but instead used from the previous manifestwork.manifest
	assert.Equal(t, oSec.Data, payloadSec.Data, "encrypt secet in payload should match the original secret is the manifestwork")

	// Test configure=T - user provided encryption secret
	configTHD.Spec.HostedClusterSpec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.AESCBC,
		AESCBC: &hyp.AESCBCSpec{
			ActiveKey: corev1.LocalObjectReference{
				Name: "test-my-etcd-encryption-key",
			},
		},
	}
	m, _ = scaffoldManifestwork(configTHD)
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 2, "2 manifestwork payload which is the hostedcluster and the generated encryption secret")
	payloadSec, _ = getManifestPayloadSecretByName(&payload, "test-my-etcd-encryption-key")
	assert.NotNil(t, payloadSec, "is not nil when encryption secret is found")
	assert.Equal(t, configTHD.Spec.HostingNamespace, payloadSec.Namespace)

	// Test configure=T - user provided activekey encryption secret not found - generate it
	configTHD.Spec.HostedClusterSpec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.AESCBC,
		AESCBC: &hyp.AESCBCSpec{
			ActiveKey: corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
		},
	}
	m, _ = scaffoldManifestwork(configTHD)
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 2, "2 manifestwork payload which is the hc & generated encryption secret")
	payloadSec, _ = getManifestPayloadSecretByName(&payload, "encryption-key-not-found")
	assert.NotNil(t, payloadSec, "is not nil when encryption secret is found")
	assert.Equal(t, configTHD.Spec.HostingNamespace, payloadSec.Namespace)

	// Test configure=T - user provided backup encryption secret not found - error
	configTHD.Spec.HostedClusterSpec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.AESCBC,
		AESCBC: &hyp.AESCBCSpec{
			ActiveKey: corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
			BackupKey: &corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
		},
	}
	m, _ = scaffoldManifestwork(configTHD)
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Len(t, err.(utilerrors.Aggregate).Errors(), 1, "backupkey encryption secret not found")

	// Test configure=F - no secret encryption
	configFHD := getHDforSecretEncryption(false)
	configFHD.Spec.HostedClusterRef = corev1.LocalObjectReference{Name: hostedCluster.Name}
	scaffoldHostedClusterSpec(configFHD)
	assert.Nil(t, configFHD.Spec.HostedClusterSpec.SecretEncryption, "secretEncryption should be nil for configure=F")
	m, err = scaffoldManifestwork(configFHD)
	assert.Nil(t, err)

	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configFHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configFHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 1, "no additonal manifestwork payload should be created, just hc")

	// Test configure=F - use provided encryption secret
	hostedCluster.Spec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.AESCBC,
		AESCBC: &hyp.AESCBCSpec{
			ActiveKey: corev1.LocalObjectReference{
				Name: "test-my-etcd-encryption-key",
			},
			BackupKey: &corev1.LocalObjectReference{
				Name: "test-my-backup-key",
			},
		},
	}
	initFakeClient(r, hostedCluster)
	m, _ = scaffoldManifestwork(configFHD)
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configFHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configFHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 3, "3 manifestwork payload which is the hc & active & backup encryption secret")
	payloadActiveSec, _ := getManifestPayloadSecretByName(&payload, userActiveKeySecret.Name)
	assert.NotNil(t, payloadActiveSec, "is not nil when encryption secret activeKey is found")
	assert.Equal(t, configFHD.Spec.HostingNamespace, payloadActiveSec.Namespace)
	payloadBackupSec, _ := getManifestPayloadSecretByName(&payload, userBackupKeySecret.Name)
	assert.NotNil(t, payloadBackupSec, "is not nil when encryption secret backupKey is found")
	assert.Equal(t, configFHD.Spec.HostingNamespace, payloadBackupSec.Namespace)

	// Test configure=F - use user encryption secret found instead of old manifestwork payload
	oActiveSecPayload := workv1.Manifest{}
	oActiveSecPayload.Raw, _ = json.Marshal(userActiveKeySecret)
	oBackkupSecPayload := workv1.Manifest{}
	oBackkupSecPayload.Raw, _ = json.Marshal(userBackupKeySecret)
	m.Spec.Workload.Manifests = []workv1.Manifest{oActiveSecPayload, oBackkupSecPayload}

	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configFHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configFHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 3, "3 manifestwork payload which is the hc & active & backup encryption secret")
	payloadActiveSec, _ = getManifestPayloadSecretByName(&payload, userActiveKeySecret.Name)
	assert.NotNil(t, payloadActiveSec, "is not nil when encryption secret activeKey is found")
	assert.Equal(t, userActiveKeySecret.Data, payloadActiveSec.Data, "active encrypt secret in payload should match the user-specified secret")
	payloadBackupSec, _ = getManifestPayloadSecretByName(&payload, userBackupKeySecret.Name)
	assert.NotNil(t, payloadBackupSec, "is not nil when encryption secret backupKey is found")
	assert.Equal(t, userBackupKeySecret.Data, payloadBackupSec.Data, "backup encrypt secret in payload should match the user-specified secret")

	// Test configure=F - activekey encryption secret not found - use secret in manifestwork
	hostedCluster.Spec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.AESCBC,
		AESCBC: &hyp.AESCBCSpec{
			ActiveKey: corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
			BackupKey: &corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
		},
	}
	initFakeClient(r, hostedCluster)

	oActiveSecPayload = workv1.Manifest{}
	userActiveKeySecret.Name = "encryption-key-not-found"
	oActiveSecPayload.Raw, _ = json.Marshal(userActiveKeySecret)
	m.Spec.Workload.Manifests = []workv1.Manifest{oActiveSecPayload}
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configFHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configFHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 3, "3 manifestwork payload which is the hc & active & backup encryption secret")
	payloadBackupSec, _ = getManifestPayloadSecretByName(&payload, userActiveKeySecret.Name)
	assert.NotNil(t, payloadBackupSec, "is not nil when encryption secret activeKey is found")
	assert.Equal(t, "encryption-key-not-found", payloadBackupSec.Name, "active encrypt secret in payload should match the user-specified secret")

	// Test configure=F - activekey encryption secret not found and not in manifestwork - fail
	hostedCluster.Spec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.AESCBC,
		AESCBC: &hyp.AESCBCSpec{
			ActiveKey: corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
			BackupKey: &corev1.LocalObjectReference{
				Name: "encryption-key-not-found",
			},
		},
	}
	initFakeClient(r, hostedCluster)
	m, _ = scaffoldManifestwork(configFHD)
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configFHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configFHD, &payload)
	assert.Len(t, err.(utilerrors.Aggregate).Errors(), 2, "2 encryption secrets (active and backup) not found")
}

func TestHDKmsEncryptionSecret(t *testing.T) {
	r := GetHypershiftDeploymentReconciler()
	ctx := context.Background()

	kmsSec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kms-key",
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}
	err := r.Create(ctx, kmsSec)
	defer r.Delete(ctx, kmsSec)
	assert.Nil(t, err, "kms encryption secret should be created with no error")

	// Test configure=T - use provided KMS encryption secret
	configTHD := getHDforSecretEncryption(true)
	scaffoldHostedClusterSpec(configTHD)
	configTHD.Spec.HostedClusterSpec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.KMS,
		KMS: &hyp.KMSSpec{
			AWS: &hyp.AWSKMSSpec{
				Auth: hyp.AWSKMSAuthSpec{
					Credentials: corev1.LocalObjectReference{
						Name: "test-kms-key",
					},
				},
			},
		},
	}
	m, err := scaffoldManifestwork(configTHD)
	assert.Nil(t, err)
	payload := []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest := r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 2, "2 manifestwork payload which is the hc & kms encryption secret")
	payloadSec, _ := getManifestPayloadSecretByName(&payload, "test-kms-key")
	assert.NotNil(t, payloadSec, "is not nil when kms secret is found")
	assert.Equal(t, configTHD.Spec.HostingNamespace, payloadSec.Namespace)

	// Test configure=T - user-specified KMS encryption secret not found, use secret in old manifestwork payload
	configTHD.Spec.HostedClusterSpec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.KMS,
		KMS: &hyp.KMSSpec{
			AWS: &hyp.AWSKMSSpec{
				Auth: hyp.AWSKMSAuthSpec{
					Credentials: corev1.LocalObjectReference{
						Name: "test-kms-key-not-found",
					},
				},
			},
		},
	}
	payloadSec.Name = "test-kms-key-not-found"
	oKmsSecPayload := workv1.Manifest{}
	oKmsSecPayload.Raw, _ = json.Marshal(payloadSec)
	m.Spec.Workload.Manifests = []workv1.Manifest{oKmsSecPayload}
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Nil(t, err)
	assert.Len(t, payload, 2, "2 manifestwork payload which is the hc & generated encryption secret")
	payloadSec, _ = getManifestPayloadSecretByName(&payload, "test-kms-key-not-found")
	assert.NotNil(t, payloadSec, "is not nil when kms secret is found")

	// Test configure=T - KMS encryption secret not found
	configTHD.Spec.HostedClusterSpec.SecretEncryption = &hyp.SecretEncryptionSpec{
		Type: hyp.KMS,
		KMS: &hyp.KMSSpec{
			AWS: &hyp.AWSKMSSpec{
				Auth: hyp.AWSKMSAuthSpec{
					Credentials: corev1.LocalObjectReference{
						Name: "test-kms-key-not-found",
					},
				},
			},
		},
	}
	m, _ = scaffoldManifestwork(configTHD)
	payload = []workv1.Manifest{}
	r.appendHostedCluster(ctx)(configTHD, &payload)
	loadManifest = r.ensureConfiguration(ctx, m)
	err = loadManifest(configTHD, &payload)
	assert.Len(t, err.(utilerrors.Aggregate).Errors(), 1, "kms encryption secrets not found")
}

// Test configmap in nodepool is added to manifestwork payload
func TestNodePoolConfigMaps(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.NodePools = []*hyd.HypershiftNodePools{}
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"

	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	// Create configmap and add it to the nodepool
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "np-test-cm",
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string]string{
			".dockerconfigjson": "docker-configmap",
		},
	}
	client.Create(ctx, cm)
	defer client.Delete(ctx, cm)

	nps := getNodePools(testHD)
	nps[0].Spec.Config = []corev1.LocalObjectReference{{Name: cm.Name}}
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	m, err := scaffoldManifestwork(testHD)
	assert.Nil(t, err)
	payload := []workv1.Manifest{}
	hdr.appendHostedCluster(ctx)(testHD, &payload)
	hdr.appendNodePool(ctx)(testHD, &payload)
	hdr.ensureConfiguration(ctx, m)(testHD, &payload)

	containsInPayload := func(wls []workv1.Manifest, cm *corev1.ConfigMap, hostingNs string) bool {
		for _, wl := range wls {
			if wl.Object.GetObjectKind().GroupVersionKind().Kind == "ConfigMap" {
				cmObj := wl.Object.(*corev1.ConfigMap)
				if cmObj.Namespace == hostingNs && cmObj.Name == cm.Name {
					return true
				}
			}
		}

		return false
	}

	// Find nodepool configmap in payload
	assert.True(t, containsInPayload(payload, cm, testHD.Spec.HostingNamespace), "true if configmap is found in the payload")

	// Test configMap in hypD.Spect.NodePools.Spec.Config
	testHD = getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"

	// Add configmap to nodepool
	testHD.Spec.NodePools[0].Spec.Config = []corev1.LocalObjectReference{{Name: cm.Name}}

	m, err = scaffoldManifestwork(testHD)
	assert.Nil(t, err)
	payload = []workv1.Manifest{}
	hdr.appendHostedCluster(ctx)(testHD, &payload)
	hdr.appendNodePool(ctx)(testHD, &payload)
	hdr.ensureConfiguration(ctx, m)(testHD, &payload)

	// Find nodepool configmap in payload
	assert.True(t, containsInPayload(payload, cm, testHD.Spec.HostingNamespace), "true if configmap is found in the payload")
}
