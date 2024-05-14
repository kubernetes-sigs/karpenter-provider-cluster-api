# Karpenter provider Cluster API

[Karpenter][karpenter] is a [Kuberneters][kubernetes] autoprovisioner that
provides just-in-time Nodes for any cluster. This repository contains an
implementation of Karpenter that uses [Cluster API][clusterapi] as the
underlying infrastructure provider. Meaning that this implementation of
Karpenter is intended to be capable of managing Nodes on any Cluster API
owned cluster.

## Status

This project is under active development and is in an experimental state.
The current focus is on implementing the [Karpenter cloudprovider interface][kci].

### cloudprovider interface implementation checklist

- [ ] `Create`
- [ ] `Delete`
- [ ] `Get`
- [ ] `List`
- [ ] `GetInstanceTypes`
- [ ] `IsDrifted`
- [ ] `Name`
- [ ] `GetSupportedNodeClasses`

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
