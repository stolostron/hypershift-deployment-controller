# Custom configuration example

## Problem Statement
Be able to deploy a fully customized OCP Hosted Control Plane.

## Concepts
The HypershiftDeployment includes the HostedClusterSpec and []NodePoolSpec. There are a number of secret and configMaps that are supported. Since the HypershiftDeployment is translated into a ManifestWork to deliver the HostedCluster definition to a remote Management Cluster, there are a few customizations that are required.

### HypershiftDeployment
* The three main credential ARN's are supported without create the initial three secrets.
  * Cloud Provider Credential
  * Control Plane Operator Credential
  * NodePool Management Credential.

These three credentials are supplied via:
```yaml
spec:
  credentials:
    aws:
      controlPlaneOperatorARN: arn:aws:iam::<ID>:role/sample-rkfrn-control-plane-operator
      kubeCloudControllerARN: arn:aws:iam::<ID>:role/sample-rkfrn-cloud-controller
      nodePoolManagementARN: arn:aws:iam::<ID>:role/sample-rkfrn-node-pool
```
This approach to containing these core ARN's in the HypershiftDeployment will save three secret resources for the standard deploy.

### HostedClusterSpec
* The minimal hostedClusterSpec is supported, but this includes the ability to have operator customizations
  * Secrets
  * ConfigMaps
  * Items  custom resources
```yaml
spec:
  hostedClusterSpec:
    sshKey:                      #OPTIONAL
        name: <SECRET06>
    hostedClusterSpec:
        configuration:
        secretRef:               #OPTIONAL
            - name: <SECRET01>
            - name: <SECRET02>
        configMapRef:            #OPTIONAL
            - name: <CONFIGMAP01>
            - name: <CONFIGMAP02>
        items:                   #OPTIONAL
            - name: <CUSTOMRESOURCE01>
            - name: <CUSTOMRESOURCE02>
        secretEncryption:          #OPTIONAL
        kms:
            aws:
            auth:
                name: <SECRET03>
        aescbc:
            activeKey:
            name: <SECRET04>
            backupKey:
            name: <SECRET05>
```
* The `hypershift-deployment-controller` will look for the references
  * If found it is copied to the ManifestWork manifest list
  * If not found, the condition status is updated with the missing secret/configmap/customResource name.
  * If delete on read annotation is present, the secret is removed after it is copied to the ManifestWork (reduce resource counts)

### NodePoolSpec
* The minimal nodePoolSpec is supported, but this includes the ability to have operator customizations.
  * ConfigMaps
```yaml
spec:
  nodePoolSpec:
    config:     
      - name: <CONFIGMAP03>
      - name: <CONFIGMAP04>
```
* The `hypershift-deployment-controller` will look for the references
  * If found it is copied to the ManifestWork manifest list
  * If not found, the condition status is updated with the missing secret/configmap/customResource name.
  * If delete on read annotation is present, the secret is removed after it is copied to the ManifestWork (reduce resource counts)