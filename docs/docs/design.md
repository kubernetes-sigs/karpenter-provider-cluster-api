# Design

As of the first half of 2024, this project is under active design and experimentation.
The documentation here captures some of the design notes and decisions.

There are several options that the Cluster API community is evaluating for the implementation of its Karpenter provider. The options are being explored based on the layer of API interaction they involve. Loosely though, these are the different approaches we have identified:

* MachineDeployment based provider. In this configuration, Karpenter will manipulate the `replicas` field of a MachineDeployment when it wants to create or delete NodeClaims. This option seeks to preserve the user decisions that are encoded into MachineDeployments to create a more Cluster API-focused experience.
* InfrastructureMachineTemplate based provider. This configuration makes use of InfrastructureMachineTemplates to create and manage ownerless Machine objects in reaction to Karpenter's requests. In this implementation, Karpenter will be directly creating and deleting Machine objects which are not owned by a scalable resource type (such as a MachineDeployment). This style of provider will require users to have a deeper knowledge of Cluster API and will eschew the features provided by MachineDeployments, and the like.
* Provider-specific based provider. This implementation reuses internal providers (e.g. instance, pricing, etc.) from other implementations (e.g. AWS, Azure) to interact with the specific cloud. Further, it then uses the information from the cloud provider to create Cluster API CRDs. The direct cloud provider interface in this implementation provides a similar experience as other Karpenter providers while also informing the user through Cluster API CRDs.

Current work in this repository is exploring the MachineDeployment option with the expectation that we might reconfigure the final implemetation depending on the Cluster API community's needs and desires.

## MachineDeployment

After the discussions on [4 April 2024][april4], we are revisiting the idea of using MachineDeployments as the backing implemetation detail for NodeClaims. The diagrams here are updated to reflect those changes.

Of note:

* The ClusterAPINodeClass will contain a label selector and assume MachineDeployment as the type.
* The label selector in the ClusterAPINodeClass will be used as the filter for finding MachineDeployments

### Implementation details

### Label for participating resources

To quickly filter Cluster API resources that are used by Karpenter, the label `node.cluster.x-k8s.io/karpenter-member` will be used on MachineDeployments and Machines.

Applying this label will be a user task and it should be added to the `.metadata.labels` and the `.spec.template.metadata.labels` of the MachineDeployment.

#### Node labels

To inform about the labels that will be on a node, the provider will translate the [Cluster API propagated labels][plabels] and the [scale-from-zero label annotations][sfza] from the MachineDeployment.

In cases where the scale-from-zero annotation is used to indicate labels, those labels will override any of the propagated labels.

Initially, this can be used to provide the zone and instance type information required by Karpenter.

#### Capacity information

To inform about the capacity of an instance type, and by extension the node it creates, the [scale-from-zero capacity annotations][sfza] will be used initially. In this manner a capacity resource list can be resolved for each scalable resource type from Cluster API.

### General resource relationships

```mermaid
classDiagram
    direction LR
    NodePool : NodeClass ref
    NodePool : ...
    ClusterAPINodeClass : ScalableResourceSelector LabelSelector
    NodePool --|> ClusterAPINodeClass
    NodeClaim : NodeClass ref
    NodeClaim --|> ClusterAPINodeClass
    NodeClaim : ...
    MachineDeployment : capacity annotations
    MachineDeploymentList : Items []MachineDeployment
    ClusterAPINodeClass --|> MachineDeploymentList : filtered list
    MachineDeploymentList --|> MachineDeployment
```

a possible workflow for creating new instances

```mermaid
sequenceDiagram
    participant Kubernetes
    participant Karpenter Core
    participant Karpenter CAPI
    participant ClusterAPI
    Kubernetes->>Karpenter Core : Pods pending or unschedulable
    Karpenter Core->>Karpenter CAPI : GetInstanceTypes()
    Karpenter CAPI->>ClusterAPI : List MachineDeployments
    ClusterAPI->>Karpenter CAPI : MachineDeploymentList
    Note left of Karpenter CAPI : convert MD capacity and<br />constraints to instance type
    Karpenter CAPI->>Karpenter Core : []Instances
    Karpenter Core->>Karpenter CAPI : Create(NodeClaim)
    Note left of Karpenter CAPI : choose correct MD<br />based on requirements
    Karpenter CAPI->>ClusterAPI : Increase Replicas
    loop Reconcile
        ClusterAPI->>Karpenter CAPI: find new Machine from MD
    end
    Note left of Karpenter CAPI: update NodeClaim status<br />with Machine info
    Karpenter CAPI->>Karpenter Core: Update NodeClaim
```

## InfrastructureMachineTemplate

The section highlights a design pattern that uses an ownerless machine methodology to create instances for NodeClaims.
This design was [presented at the 4 April 2024 office hours][april4] - [slides](assets/Proof of Concept Architecture for Karpenter Cluster API.pdf).

This is a class diagram showing the relationships between the Karpenter
and Cluster API CRDs.

```mermaid
classDiagram
    direction LR
    NodePool : NodeClass ref
    NodePool : ...
    ClusterAPINodeClass : MachineTemplate ref
    ClusterAPINodeClass : Labels []string
    NodePool --|> ClusterAPINodeClass
    NodeClaim : NodeClass ref
    NodeClaim --|> ClusterAPINodeClass
    NodeClaim : ...
    MachineTemplate : status.capacity ResourceList
    MachineTemplateList : Items []MachineTemplate
    ClusterAPINodeClass --|> MachineTemplateList : filtered list
    MachineTemplateList --|> MachineTemplate
```

This is the sequence diagram for what a request to create a new Node might
look like.

```mermaid
sequenceDiagram
    participant Kubernetes
    participant Karpenter Core
    participant Karpenter CAPI
    participant ClusterAPI
    Kubernetes->>Karpenter Core : Pods pending or unschedulable
    Karpenter Core->>Karpenter CAPI : GetInstanceTypes()
    Karpenter CAPI->>ClusterAPI : List MachineTemplates
    ClusterAPI->>Karpenter CAPI : MachineTemplateList
    Note left of Karpenter CAPI : convert template capacity and<br />constraints to instance type
    Karpenter CAPI->>Karpenter Core : []Instances
    Karpenter Core->>Karpenter CAPI : Create(NodeClaim)
    Note left of Karpenter CAPI : choose correct template<br />based on requirements
    Karpenter CAPI->>ClusterAPI : Create Machine
    Note left of Karpenter CAPI: reconcile Machine to NodeClaim status
    Karpenter CAPI->>Kubernetes: Update NodeClaim
```

## Provider-specific

This implementation design reuses portions of other Karpenter implementations to provide the most direct experience with the underlying infrastructure. It creates, updates, or deletes Cluster API CRDs based on the actions of Karpenter and the cloud provider. This implementation has the closest experience to a native Karpenter while still providing the Cluster API experience through CRDs which reflect the actions that have been performed by Karpenter.

### Implementation detals

The [following commit demonstrates a foundation][commit] towards this pattern (thank you to community member @enxebre). It is a starting point to predict the design direction for this implementation.


[april4]: https://youtu.be/xINYfl5j8WI?si=PiWu7MeaXy3SWGKX&t=1281
[sfz]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210310-opt-in-autoscaling-from-zero.md
[plabels]: https://cluster-api.sigs.k8s.io/developer/architecture/controllers/metadata-propagation.html?highlight=metadata#metadata-propagation
[sfza]: https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/clusterapi#scale-from-zero-support
[commit]: https://github.com/enxebre/karpenter-provider-cluster-api/commit/3a8b1147ad4df8874a1a9e1eb4f1c0d177b6b8c7
