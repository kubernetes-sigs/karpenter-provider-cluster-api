# Design

As of the first half of 2024, this project is under active design and experimentation.
The documentation here captures some of the design notes and decisions.

## MachineDeployment

After the discussions on [4 April 2024][april4], we are revisiting the idea of using MachineDeployments as the backing implemetation detail for NodeClaims. The diagrams here are updated to reflect those changes.

Of note:

* The ClusterAPINodeClass will contain a reference to a type of MachineDeployment not a specific resource.
* The labels in the ClusterAPINodeClass will be used as inclusive filters for finding MachineDeployments
* The capacity annotations on the MachineDeployment are the same as the [scale from zero][sfz] annotations.

general resource relationships

```mermaid
classDiagram
    direction LR
    NodePool : NodeClass ref
    NodePool : ...
    ClusterAPINodeClass : MachineDeployment ref
    ClusterAPINodeClass : Labels []string
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

The section highlights a design pattern that uses an orphan machine methodology to create instances for NodeClaims.
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


[april4]: https://youtu.be/xINYfl5j8WI?si=PiWu7MeaXy3SWGKX&t=1281
[sfz]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210310-opt-in-autoscaling-from-zero.md
