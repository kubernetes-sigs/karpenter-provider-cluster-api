# Settings

[comment]: <> (the content below is generated from hack/docs/settings_gen/main.go)

Karpenter exposes environment variables and CLI flags that allow you to configure controller behavior. The available settings are outlined below.

| Environment Variable | CLI Flag | Description |
|--|--|--|
| BATCH_IDLE_DURATION | \-\-batch-idle-duration | The maximum amount of time with no new pending pods that if exceeded ends the current batching window. If pods arrive faster than this time, the batching window will be extended up to the maxDuration. If they arrive slower, the pods will be batched separately. (default = 1s)|
| BATCH_MAX_DURATION | \-\-batch-max-duration | The maximum length of a batch window. The longer this is, the more pods we can consider for provisioning at one time which usually results in fewer but larger nodes. (default = 10s)|
| CLUSTER_API_CERTIFICATE_AUTHORITY_DATA | \-\-cluster-api-certificate-authority-data | The cert certificate authority of the cluster api manager cluster|
| CLUSTER_API_KUBECONFIG | \-\-cluster-api-kubeconfig | The path to the cluster api manager cluster kubeconfig file.  Defaults to service account credentials if not specified.|
| CLUSTER_API_SKIP_TLS_VERIFY | \-\-cluster-api-skip-tls-verify | Skip the check for certificate for validity of the cluster api manager cluster. This will make HTTPS connections insecure|
| CLUSTER_API_TOKEN | \-\-cluster-api-token | The Bearer token for authentication of the cluster api manager cluster|
| CLUSTER_API_URL | \-\-cluster-api-url | The url of the cluster api manager cluster|
| DISABLE_LEADER_ELECTION | \-\-disable-leader-election | Disable the leader election client before executing the main loop. Disable when running replicated components for high availability is not desired.|
| ENABLE_PROFILING | \-\-enable-profiling | Enable the profiling on the metric endpoint|
| FEATURE_GATES | \-\-feature-gates | Optional features can be enabled / disabled using feature gates. Current options are: NodeRepair, ReservedCapacity, and SpotToSpotConsolidation (default = NodeRepair=false,ReservedCapacity=false,SpotToSpotConsolidation=false)|
| HEALTH_PROBE_PORT | \-\-health-probe-port | The port the health probe endpoint binds to for reporting controller health (default = 8081)|
| KARPENTER_SERVICE | \-\-karpenter-service | The Karpenter Service name for the dynamic webhook certificate|
| KUBE_CLIENT_BURST | \-\-kube-client-burst | The maximum allowed burst of queries to the kube-apiserver (default = 300)|
| KUBE_CLIENT_QPS | \-\-kube-client-qps | The smoothed rate of qps to kube-apiserver (default = 200)|
| LEADER_ELECTION_NAME | \-\-leader-election-name | Leader election name to create and monitor the lease if running outside the cluster (default = karpenter-leader-election)|
| LEADER_ELECTION_NAMESPACE | \-\-leader-election-namespace | Leader election namespace to create and monitor the lease if running outside the cluster|
| LOG_ERROR_OUTPUT_PATHS | \-\-log-error-output-paths | Optional comma separated paths for logging error output (default = stderr)|
| LOG_LEVEL | \-\-log-level | Log verbosity level. Can be one of 'debug', 'info', or 'error' (default = info)|
| LOG_OUTPUT_PATHS | \-\-log-output-paths | Optional comma separated paths for directing log output (default = stdout)|
| MEMORY_LIMIT | \-\-memory-limit | Memory limit on the container running the controller. The GC soft memory limit is set to 90% of this value. (default = -1)|
| METRICS_PORT | \-\-metrics-port | The port the metric endpoint binds to for operating metrics about the controller itself (default = 8080)|
| PREFERENCE_POLICY | \-\-preference-policy | How the Karpenter scheduler should treat preferences. Preferences include preferredDuringSchedulingIgnoreDuringExecution node and pod affinities/anti-affinities and ScheduleAnyways topologySpreadConstraints. Can be one of 'Ignore' and 'Respect' (default = Respect)|
