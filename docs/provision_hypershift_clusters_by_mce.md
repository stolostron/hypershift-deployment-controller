# Provision Hypershift Clusters by MCE

The multicluster-engine(MCE) has been installed and at least one OCP managed cluster(e.g. `hypershift-management-cluster`, If you want the hub cluster to act as a hypershift management cluster, you can also use `local-cluster`) has been imported. We will make this OCP managed cluster a hypershift management cluster.

## Turn one of the managed clusters into the hypershift management cluster

We call the cluster with the hypershift operator installed as the hypershift management cluster. In this section, we will use hypershift-addon to install a hypershift operator to one of the managed cluster.

1. Create ManagedClusterAddon hypershift-addon
```
╰─$ oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: hypershift-addon
  namespace: hypershift-management-cluster # the managed OCP cluster you want to install hypershift operator
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

2. Create an oidc s3 credentials secret for the hypershift operator, name is `hypershift-operator-oidc-provider-s3-credentials` in the `hypershift-management-cluster` namespace, which one you want to install hypershift operator.

The secret must contain 3 fields:
- `bucket`: An S3 bucket with public access to host OIDC discovery documents for your hypershift clusters
- `credentials`: region of the bucket
- `region`: credentials to access the bucket

For details, please check: https://hypershift-docs.netlify.app/getting-started/ , you can create this secret by:
```
oc create secret generic hypershift-operator-oidc-provider-s3-credentials --from-file=credentials=$HOME/.aws/credentials --from-literal=bucket=<s3-bucket-for-hypershift> --from-literal=region=<region> -n <hypershift-management-cluster>
```

3. Check the hypershift-addon is installed
```
╰─# oc get managedclusteraddons -n local-cluster hypershift-addon
NAME               AVAILABLE   DEGRADED   PROGRESSING
hypershift-addon   True
```

## Provision a hypershift hosted cluster

After the hypershift operator is installed, we can provision a hypershift hosted cluster by `HypershiftDeployment`

1. Create a cloud provider secret, it has the following format for AWS:
```
  apiVersion: v1
  metadata:
      name: my-aws-cred
      namespace: default      # Where you will create HypershiftDeployment resources
  type: Opaque
  kind: Secret
  stringData:
      ssh-publickey:          # Value
      ssh-privatekey:         # Value
      pullSecret:             # Value, required
      baseDomain:             # Value, required
      aws_secret_access_key:  # Value, required
      aws_access_key_id:      # Value, required
```

You can create this secret by:
- ACM console: https://<Advanced-Cluster-Management-Console>/credentials/add

or
- oc command
```
oc create secret generic <my-secret> -n <hypershift-deployment-namespace> --from-literal=baseDomain='your.domain.com' --from-literal=aws_access_key_id='your-aws-access-key' --from-literal=aws_secret_access_key='your-aws-secret-key' --from-literal=pullSecret='{"auths":{"cloud.openshift.com":{"auth":"auth-info", "email":"xx@redhat.com"}, "quay.io":{"auth":"auth-info", "email":"xx@redhat.com"} } }' --from-literal=ssh-publickey='your-ssh-publickey' --from-literal=ssh-privatekey='your-ssh-privatekey'
```

2. Create a HypershiftDeployment in the cloud provider secret namespace
```
╰─$ oc apply -f - <<EOF                                                                                                          130 ↵
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: hypershift-demo
  namespace: default
spec:
  targetManagedCluster: hypershift-management-cluster
  targetNamespace: clusters       # issue see https://github.com/stolostron/hypershift-deployment-controller/issues/30
  override: MANIFESTWORK
  infrastructure:
    cloudProvider:
      name: <my-secret>
    configure: True
    platform:
      aws:
        region: <region>
EOF
```

3. Check the HypershiftDeployment status
```
oc get hypershiftdeployment -n default hypershift-demo -w
```

4. After the hosted cluster is created, it will be imported to the hub automatically, you can check it with:
```
oc get managedcluster <hypershiftDeployment.Spec.infraID>
```

## Access the hosted cluster

The access secrets are stored in the {hypershift-management-cluster} namespace.
The formats of the secrets name are:
- kubeconfig secret: `<hypershiftDeployment.Spec.targetNamespace>-<hypershiftDeployment.Name>-admin-kubeconfig` (e.g clusters-hypershift-demo-admin-kubeconfig)
- kubeadmin password secret: `<hypershiftDeployment.Spec.targetNamespace>-<hypershiftDeployment.Name>-kubeadmin-password` (e.g clusters-hypershift-demo-kubeadmin-password)

## Destroying your hypershift Hosted cluster

Delete the HypershiftDeployment resource
```
oc delete hypershiftdeployment hypershift-demo -n default
```

## Destroying hypershift operator

Delete the hypershift-addon
```
oc delete managedclusteraddon -n <hypershift-management-cluster> hypershift-addon
```
