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

**cloudprovider interface implementation checklist**

- [ ] `Create`
- [ ] `Delete`
- [ ] `Get`
- [ ] `List`
- [ ] `GetInstanceTypes`
- [ ] `IsDrifted`
- [ ] `Name`
- [ ] `GetSupportedNodeClasses`


[karpenter]: https://karpenter.sh
[kubernetes]: https://kubernetes.io
[clusterapi]: https://cluster-api.sigs.k8s.io
[kci]: https://github.com/kubernetes-sigs/karpenter/blob/main/pkg/cloudprovider/types.go 
