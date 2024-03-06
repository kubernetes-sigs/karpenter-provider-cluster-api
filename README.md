# Karpenter provider Cluster API

[Karpenter][karpenter] is a [Kuberneters][kubernetes] autoprovisioner that
provides just-in-time Nodes for any cluster. This repository contains an
implementation of Karpenter that uses [Cluster API][clusterapi] as the
underlying infrastructure provider. Meaning that this implementation of
Karpenter is intended to be capable of managing Nodes on any Cluster API
owned cluster.


[karpenter]: https://karpenter.sh
[kubernetes]: https://kubernetes.io
[clusterapi]: https://cluster-api.sigs.k8s.io
