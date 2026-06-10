# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Karpenter Provider for Cluster API** is an experimental Kubernetes controller implementation that allows Karpenter (a Kubernetes autoprovisioner) to manage nodes on clusters using Cluster API. This provider acts as the infrastructure integration layer between Karpenter and Cluster API, enabling Karpenter to provision and deprovision nodes through Cluster API's MachineDeployment resources.

**Current Status**: Experimental/PoC (v0.2.0). The main branch is unstable as work continues toward compatibility with latest Karpenter and Cluster API versions.

## Build & Development Commands

### Build
```bash
make build                    # Build the main controller binary (bin/karpenter-clusterapi-controller)
make manifests              # Generate CRDs and RBAC manifests from controller-gen
make generate               # Generate all controller-gen related objects
make gen-objects            # Generate deepcopy and other controller-gen objects
```

### Testing
```bash
make test                   # Run all unit tests (vendors deps, downloads kubebuilder assets, runs Ginkgo)
make unit                   # Run just the unit tests (faster, assumes assets are cached)
```

To run a single test package:
```bash
GINKGO_EXTRA_ARGS="-focus <test-name-pattern>" make unit
```

### Container & Deployment
```bash
make image-build            # Build and push container image using buildx
make image-push             # Build and push image to IMG_REGISTRY
make apply                  # Deploy controller to cluster (requires ko, kubeconfig, and management cluster kubeconfig configmap)
make build-with-ko          # Build image using ko (stores in KWOK_REPO)
```

### Documentation & Releases
```bash
make docgen                 # Generate documentation from code (outputs to docs/)
make release                # Create release branch and update chart version
```

### Configuration
Key Make variables can be overridden:
- `IMG_REGISTRY`: Container registry (default: `gcr.io/k8s-staging-karpenter-cluster-api`)
- `IMG_NAME`: Image name (default: `karpenter-clusterapi-controller`)
- `IMG_TAG`: Image tag (default: git describe --tags --dirty --always)
- `CONTAINER_RUNTIME`: `docker` or `podman` (default: `docker`)
- `ARCH`: Build architecture (default: output of `go env GOARCH`)
- `KWOK_REPO`: Repository for ko builds (default: `kind.local`)

## Architecture & Key Components

### High-Level Design
The provider implements Karpenter's `CloudProvider` interface to allow Karpenter's provisioning and deprovisioning logic to work with Cluster API resources. The flow:
1. **Karpenter Core** (running in the target cluster) reconciles NodePools and sends NodeClaims to the CloudProvider
2. **This Provider** (CloudProvider implementation) translates NodeClaims into Cluster API MachineDeployments and Machines
3. **Cluster API Infrastructure Providers** handle the actual infrastructure-specific provisioning (e.g., AWS, Azure, vSphere)

### Directory Structure

**`pkg/cloudprovider/`** - Core CloudProvider Implementation
- `cloudprovider.go`: Main CloudProvider implementation (handles Create, Delete, Get operations on nodes)
- `cloudprovider_test.go`: Unit tests for CloudProvider
- `const.go`: Constants like label names, annotations
- `suite_test.go`: Ginkgo test suite setup

**`pkg/providers/`** - Provider Abstractions
- `providers.go`: Provider registration and factory
- `machinedeployment/`: Abstractions for working with Cluster API MachineDeployments
  - `machinedeployment.go`: MachineDeployment operations and discovery
  - `machinedeployment_test.go`: Unit tests
- `machine/`: Abstractions for working with Cluster API Machines
  - `machine.go`: Individual Machine operations

**`pkg/controllers/`** - Kubernetes Controllers
- `controllers.go`: Controller registration
- `nodeclass/`: NodeClass resource controller(s)

**`pkg/apis/`** - Custom Resource Definitions
- `v1alpha1/`: ClusterAPINodeClass (custom CRD for this provider)
  - `clusterapinodeclass.go`: API type definitions
  - Labels and registration
- `crds/`: Generated CRD manifests (output of `make manifests`)

**`pkg/operator/`** - Operator Initialization
- Custom operator setup wrapping controller-runtime
- `options/`: Command-line flags and configuration

**`pkg/test/`** - Test Utilities
- `util.go`: Shared testing helpers

**`cmd/controller/`** - Entry Point
- `main.go`: Bootstrap the operator with core Karpenter controllers, this provider's controllers, and the CloudProvider

**`test/`** - Test Resources & Scripts
- `resources/`: YAML manifests for testing deployments
- `hack/`: Test setup scripts

**`hack/`** - Development Utilities
- `docgen.sh`: Generate docs from code comments
- `release.sh`: Release automation
- `boilerplate.go.txt`, `boilerplate.sh`: License header templates

## Key Dependencies & Versions

- **Go**: 1.24.2
- **Karpenter**: v1.4.0 (provides CloudProvider interface, core controllers, state tracking)
- **Cluster API**: v1.10.10 (provides Machine, MachineDeployment, infrastructure template CRDs)
- **controller-runtime**: v0.21.0 (Kubernetes operator framework)
- **Test Framework**: Ginkgo v2, Gomega for BDD-style tests
- **Code Generation**: controller-tools v0.16.5 for CRD and RBAC generation

## Key Design Patterns

### Multi-Cluster Configuration
Unlike single-cluster Karpenter providers, this provider must manage clients for multiple clusters in hub/spoke topologies:
- Management cluster: where MachineDeployment/Machine resources live
- Target cluster(s): where Nodes, Pods, and Karpenter run

### MachineDeployment-to-Machine Translation
NodeClaims are translated into Machines derived from MachineDeployments. The provider:
1. Discovers available MachineDeployments (filtered by label/annotation)
2. Creates Machines from their InfrastructureTemplate
3. Tracks which Machines belong to Karpenter (via labels)

### Resource Identification
Cluster API resources are identified for Karpenter use via labels/annotations. Karpenter-managed Machines are marked with specific labels to distinguish them from manually created ones.

## Testing Approach

Tests use Ginkgo v2 with `envtest` for integration with a temporary Kubernetes API server:
- `ENVTEST_K8S_VERSION`: Version of kubebuilder assets (currently 1.32.0)
- Tests run with race detection (`--race`), randomization, and timeouts
- Cloud provider tests include mocking Cluster API resources

## Disruption & Consolidation

Karpenter's disruption framework (Emptiness, SingleNodeConsolidation, MultiNodeConsolidation) is **already wired up** and functional. The provider supports:

### Emptiness-Based Disruption
- Automatically removes nodes with no running pods
- Works out-of-the-box with no configuration

### Cost-Aware Consolidation
- Requires setting price annotations on MachineDeployments (see below)
- Consolidation engine compares node costs when deciding replacements
- Helps optimize cloud costs by preferring cheaper instance types

### Safety Guards

**Minimum Replica Count Protection**
- Prevent scaling below the configured minimum
- Set via `cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size` annotation on MachineDeployment
- Consolidation respects these bounds and won't drain a MD below its minimum

**Paused MachineDeployment Handling**
- MachineDeployments with `spec.paused: true` are excluded from consolidation
- New nodes won't be created on paused MDs
- Allows for safe updates without disruption

### Operator Configuration

**Price Annotation** (for cost-aware consolidation)
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: example-md
  annotations:
    capacity.cluster-autoscaler.kubernetes.io/price: "0.50"  # hourly cost
spec:
  # ...
```

**Min/Max Size Annotations** (for scaling bounds)
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: example-md
  annotations:
    cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size: "1"
    cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size: "10"
spec:
  replicas: 3
  # ...
```

### Instance Type Discovery

The provider requires one of:
1. **Instance Type Label** on MachineDeployment template (propagated to Nodes)
   ```yaml
   spec:
     template:
       labels:
         node.cluster.x-k8s.io/instance-type: "standard"
   ```
2. **MachineDeployment Name** (used as fallback if label absent)
   - Disruption engine matches nodes by instance type name
   - Ensure your cloud provider labels Nodes with `node.kubernetes.io/instance-type`

## Development Notes

- **Code Generation**: Run `make generate` before committing if you modify API types or RBAC requirements
- **CRD Updates**: Changes to `pkg/apis/` trigger CRD regeneration via `make manifests`
- **Vendoring**: Dependencies are vendored; use `make vendor` to update
- **Controller-Runtime Pattern**: Controllers follow standard kubebuilder/controller-runtime patterns with Reconcile loops
- **Integration Points**: The CloudProvider directly integrates Karpenter's provisioning logic with Cluster API's resource model
- **Provider Interface**: The `machinedeployment.Provider` interface includes both `Update()` (for general updates) and `Scale()` (for replica changes with patch-based resilience)
