# Running MCE addons in hosted mode along with hosted control plane

When a hypershift hosted cluster is imported as a MCE managed cluster, MCE addons are installed on the hosted cluster on its worker nodes by default (default mode). There is an option to install the MCE addons along with the hosted control plane on the hosting cluster (hosted mode) for cases where the hosted cluster has no worker node or for other logistic reasons. Running the addons in hosted mode requires some extra configurations in `ManagedCluster`, creating kubeconfig secrets for the addons, and manually installing the addons in hosted mode.

`hosting cluster` : is the MCE managed cluster where hypershift hosted clusters are created.
`hosted cluster`: is the hypershift hosted clusters you create.

## Importing a hosted cluster as a ManagedCluster in hosted mode with no addon

Create a ManagedCluster on the MCE hub cluster with these four annotations to indicate that the managed cluster (hypershift hosted cluster) is imported in hosted mode with no MCE addon.

```bash
$ oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: HOSTED-CLUSTER-NAME
  Annotations:
    import.open-cluster-management.io/klusterlet-deploy-mode: Hosted
    import.open-cluster-management.io/hosting-cluster-name: HOSTING-CLUSTER-NAME
    import.open-cluster-management.io/klusterlet-namespace: klusterlet-HOSTED-CLUSTER-NAME
    addon.open-cluster-management.io/disable-automatic-installation: "true"
spec:
  hubAcceptsClient: true
  leaseDurationSeconds: 60
EOF
```

Replace `HOSTED-CLUSTER-NAME` with your hypershift hosted cluster name. There are two places: the managed cluster name and the klusterlet-namespace annotation. `HOSTING-CLUSTER-NAME` is the MCE managed cluster name where you provision your hypershift hosted cluster. You can add other additional labels and annotations. 

You can create the hypershift hosted cluster before or after creating this `ManagedCluster`. See https://github.com/rokej/hypershift-deployment-controller/blob/main/docs/provision_hypershift_clusters_by_manifestwork.md for creating it. 

**Important**: You cannot create the hypershift hosted cluster using `HypershiftDeployment` because it will remove these special annotations and install the MCE addons in default mode.

Run the following command to check the managed cluster status and wait until it is `HUB ACCEPTED`, `JOINED` and `AVAILABLE`.

```bash
$ oc get managedcluster HOSTED-CLUSTER-NAME
```

## Enable work manager addon in hosted mode

1. Log into the MCE hub cluster and create `ManagedClusterAddon` to enable work manager addon.

    ```bash
    $ oc apply -f - <<EOF
    apiVersion: addon.open-cluster-management.io/v1alpha1
    kind: ManagedClusterAddOn
    metadata:
    name: work-manager
    namespace: HOSTED-CLUSTER-NAME
    annotations:
        addon.open-cluster-management.io/hosting-cluster-name: HOSTING-CLUSTER-NAME
    spec:
    installNamespace: ADDON-INSTALL-NAMESPACE
    EOF
    ```

    Replace `HOSTED-CLUSTER-NAME` with your hypershift hosted cluster name. `HOSTING-CLUSTER-NAME` is the MCE managed cluster name where you provision your hypershift hosted cluster.

    The annotation in this `ManagedClusterAddon` indicates that the addon is installed in hosted mode on the hosting cluster. Choose a unique `ADDON-INSTALL-NAMESPACE` for each hypershift hosted cluster.

2. Log into the `hosting cluster`.

3. Look for `admin-kubeconfig` secret in the hosted control plane namespace. This is normally `clusters-HOSTED-CLUSTER-NAME`.

4. Copy the secret into `ADDON-INSTALL-NAMESPACE` namespace and change the secret name to `work-manager-managed-kubeconfig`.

5. Use the following command on the MCE hub cluster to check the status of the work manager addon and ensure it is `AVAILABLE`.

    ```bash
    $ oc get managedclusteraddon work-manager -n HOSTED-CLUSTER-NAME
    ```

## Enable policy addons in hosted mode

1. Log into the MCE hub cluster and create `ManagedClusterAddon` to enable work manager addon.

    ```bash
    $ oc apply -f - <<EOF
    apiVersion: addon.open-cluster-management.io/v1alpha1
    kind: ManagedClusterAddOn
    metadata:
    name: config-policy-controller
    namespace: HOSTED-CLUSTER-NAME
    annotations:
        addon.open-cluster-management.io/hosting-cluster-name: HOSTING-CLUSTER-NAME
    spec:
    installNamespace: ADDON-INSTALL-NAMESPACE
    ---
    apiVersion: addon.open-cluster-management.io/v1alpha1
    kind: ManagedClusterAddOn
    metadata:
    name: governance-policy-framework
    namespace: HOSTED-CLUSTER-NAME
    annotations:
        addon.open-cluster-management.io/hosting-cluster-name: HOSTING-CLUSTER-NAME
    spec:
    installNamespace: ADDON-INSTALL-NAMESPACE
    EOF
    ```

    Replace `HOSTED-CLUSTER-NAME` with your hypershift hosted cluster name. `HOSTING-CLUSTER-NAME` is the MCE managed cluster name where you provision your hypershift hosted cluster.

    The annotation in this `ManagedClusterAddon` indicates that the addon is installed in hosted mode on the hosting cluster. Choose a unique `ADDON-INSTALL-NAMESPACE` for each hypershift hosted cluster. It can be the same namespace as the work manager addon you previously enabled.

2. Log into the `hosting cluster`.

3. Look for `admin-kubeconfig` secret in the hosted control plane namespace. This is normally `clusters-HOSTED-CLUSTER-NAME`.

4. Copy the secret into `ADDON-INSTALL-NAMESPACE` namespace and change the secret name to `config-policy-controller-managed-kubeconfig`.

5. Use the following command on the MCE hub cluster to check the status of the work manager addon and ensure it is `AVAILABLE`.

    ```bash
    $ oc get managedclusteraddon config-policy-controller -n HOSTED-CLUSTER-NAME
    ```