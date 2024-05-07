# Design

As of the first half of 2024, this project is under active design and experimentation.
The documentation here captures some of the design notes and decisions.

## InfrastructureMachineTemplate

The section highlights a design pattern that uses an orphan machine methodology to create instances for NodeClaims.
This design was [presented at the 4 April 2024 office hours](https://youtu.be/xINYfl5j8WI?si=PiWu7MeaXy3SWGKX&t=1281)
- [slides](assets/Proof of Concept Architecture for Karpenter Cluster API.pdf).

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
