# Karpenter provider Cluster API

[Karpenter][karpenter] is a [Kuberneters][kubernetes] autoprovisioner that
provides just-in-time Nodes for any cluster. This repository contains an
implementation of Karpenter that uses [Cluster API][clusterapi] as the
underlying infrastructure provider. Meaning that this implementation of
Karpenter is intended to be capable of managing Nodes on any Cluster API
owned cluster.

**What is a "provider"?**

The term "provider" is used in different ways depending on the context. Within the
Cluster API frame of reference, a "provider" is the controllers and CRDs
which form the integration between the Cluster API Kubernetes resources (for example Machine,
MachineDeployment, etc.) and the underlying infrastructure (for example OpenStack).
Within the Karpenter frame of reference, a "provider" is the implementation of Karpenter
for a specific infrastructure.

This repository contains the specific implementation of Karpenter that can interact with Kubernetes
clusters which contain the Cluster API CRDs and controllers. In this manner, Cluster API
is the infrastructure to which this Karpenter implementation can interact with, in much the
same way as other Karpenter implementations may interact directly with AWS or Azure.

## Status

This project is an experimental proof of concept for how Karpenter might integrate with Cluster API.
The intention is for the Kubernetes community to use this experiment for learning more about the
design pattern, feature gaps, and areas for improvement. With the ultimate goal being a standard release
cycle and production ready Karpenter Cluster API provider.

The `v0.1.0` release contains basic functionality for creating and deleting nodes within a cluster.
Please see the [quickstart instructions for more details](docs/docs/getting-started.md).

The `main` branch is potentially unstable as we work towards compatibility with the latest versions
of Karpenter and Cluster API. The following features are under development or in some working state:

- [x] create nodes
- [x] delete nodes
- [ ] drift detection
- [ ] disruption/consolidation
- [ ] cost integration

For information about how to build and run the Karpenter Cluster API provider, please
see the [Getting Started](docs/docs/getting-started.md) documentation.

### Design topics

These are a few topics that are under active discussion by the
[Cluster API Karpenter Feature Group][cakfg] which are impacting
the architecture and implementation of this provider.

* **Multi-cluster client configuration.**
  For Cluster Api, when using clusters in a hub/spoke topology multiple configurations
  are required to access all the Kubernetes objects. This poses a design issue when
  using Karpenter as it assumes a single cluster, and single configuration. A common
  deployment pattern will be to have Cluster API Machine resources in the same cluster
  as the Karpenter NodePool resources, with the Node and Pod objects residing in a
  different cluster.
* **Tracking Machines through scalable resource**
  As an initial design approach, this provider will interface with MachineDeployment
  resource from Cluster API. The intention is to preserve the experience and behavioral
  configurations encoded in MachineDeployments to help guide the creation of Machines
  from NodeClaims. This does create some unique issues when translating between replica
  changes in the MachineDeployment to specific Machines that are created.
* **Identification of Cluster API resources participating with Karpenter.**
  There are several types of Cluster API resources which can participate in Karpenter
  provisioning, from the infrastructure templates to the machines. Users will need to
  indicate which infrastructure templates can be utilized by Karpenter, and likewise
  which are the machines created by Karpenter.
* **Exposure of price data.**
  Cluster API does not have a mechanism for providers to expose data about the cost of
  specific instance types. Having this information will ultimately be essential to
  unlocking the cost saving features of Karpenter.

[karpenter]: https://karpenter.sh
[kubernetes]: https://kubernetes.io
[clusterapi]: https://cluster-api.sigs.k8s.io
[kci]: https://github.com/kubernetes-sigs/karpenter/blob/main/pkg/cloudprovider/types.go
[cakfg]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/community/20231018-karpenter-integration.md
