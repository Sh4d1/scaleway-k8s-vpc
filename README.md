# Scaleway K8S VPC

**Note**: This in just a Proof of Concept, it is not suited for production usage.

Scaleway K8S VPC is a controller for Kubernetes running on Scaleway, leveraging CRDs to use PrivateNetwork in the cluster.

## Getting started

Install the controller and the node daemon with:
```yaml
kubectl create -k https://github.com/Sh4d1/scaleway-k8s-vpc/config/default
```

Create and enter your Scaleway credentials with:
```yaml
kubectl create -f secret.yaml --edit
```

You can now create the following PrivateNetwork object:
```yaml
apiVersion: vpc.scaleway.com/v1alpha1
kind: PrivateNetwork
metadata:
  name: my-privatenetwork
spec:
  id: <private network ID>
  cidr: 192.168.0.0/24
  routes:
  - to: 1.2.3.4/16
    via: 192.168.0.10
```

This will attach the private network to all nodes in the cluster, set up the interfaces with IPs in the range, and add the routes if needed.

## Contribution

Feel free to submit any issue, feature request or pull request :smile:!
