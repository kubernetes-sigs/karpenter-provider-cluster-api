# karpenter

A Helm chart for Kubernetes that provides an implementation of Karpenter using Cluster API as the infrastructure
provider

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.2.0](https://img.shields.io/badge/AppVersion-0.2.0-informational?style=flat-square)

## Documentation

For full Karpenter documentation please checkout [https://karpenter.sh](https://karpenter.sh/docs/).

## Installing the Chart

```bash
helm upgrade --install --namespace karpenter --create-namespace karpenter .
```

## Values

| Key              | Type   | Default                             | Description                                                       |
|------------------|--------|-------------------------------------|-------------------------------------------------------------------|
| affinity         | object | `{}`                                | Affinity rules for scheduling the pod                             |
| arguments        | list   | `[]`                                | Arguments for the controller                                      |
| fullnameOverride | string | `""`                                | Overrides the chart's computed fullname                           |
| image.pullPolicy | string | `"IfNotPresent"`                    | Image pull policy of the controller image                         |
| image.tag        | string | `"karpenter-clusterapi-controller"` | Tag of the controller image                                       |
| nameOverride     | string | `""`                                | Overrides the chart's name                                        |
| nodeSelector     | object | `{}`                                | Node selectors to schedule the pod to nodes with labels           |
| replicaCount     | int    | `1`                                 | Number of replicas                                                |
| resources        | object | `{}`                                | Resources for the controller container                            |
| tolerations      | list   | `[]`                                | Tolerations to allow the pod to be scheduled to nodes with taints |
| volumeMounts     | list   | `[]`                                | VolumeMounts for the controller container                         |
| volumes          | list   | `[]`                                | Volumes for the pod                                               |
