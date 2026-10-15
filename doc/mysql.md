# MySQL

`freno` was written to assist in controlling writes to `MySQL` clusters.

### Background

`MySQL` installments typically include a master and multiple replicas. Aggressive write to the master may cause increased replication lags. Replication lags have multiple undesired effects, such as stale replica data.

Common operations apply massive changes to MySQL data, such as:

- Archiving/purging of old data (e.g. via `pt-archiver`)
- Online migrations (e.g. via `gh-ost`)
- Population of newly added columns
- Bulk loading of data (e.g. importing data from Hadoop via `sqoop`)
- Any application generated massive update

Such operations can and should be broken to smaller subtasks (e.g. `100` rows at a time) and throttle based on replication lag.

Tools such as `gh-ost` and `pt-archiver` already support auto-throttling by replication lag, however:

- They use different internal implementations
- They need to be _given_ the list of servers
- They do not adapt automatically to a change in the list of relevant servers (`gh-ost` can be updated dynamically, but again must be _told_ the identities of the replicas)

`freno` provides a unified service which self-adapts to changes in MySQL replica inventory.

### Configuration

Let's dissect the MySQL part of the [sample config file](../resources/freno.conf.sample.json):

You will find the top-level configuration:

```json
"MySQL": {
  "User": "some_user",
  "Password": "${mysql_password_env_variable}",
  "MetricQuery": "select unix_timestamp(now(6)) - unix_timestamp(ts) as lag_check from meta.heartbeat order by ts desc limit 1",
  "CacheMillis": 0,
  "ThrottleThreshold": 1.0,
  "IgnoreHostsCount": 0,
  "IgnoreHostsThreshold": 0.0,
  "HttpCheckPort": -1,
  "HttpCheckPath": "path-to-check",
  "IgnoreHosts": [
    "us-east-1",
    "us-east-2"
  ],
  "FallbackCluster": "",
  "Clusters": {
  }
}
```

These params apply in general to all MySQL clusters, unless specified differently (overridden) on a per-cluster basis.

- `User`, `Password`: these can be specified as plaintext, or in a `${some_env_variable}` format, in which case `freno` will look up its environment for specified variable. (e.g. to match the above config, a `shell` script invoking `freno` can `export mysql_password_env_variable=flyingcircus`)
- `MetricQuery`:
  - Note: returned value is expected to be `[0..)` (`0` or more), where lower values are "better" and higher values are "worse".
  - if not provided, `freno` will assume you're interested in replication lag, and will issue a `SHOW SLAVE STATUS` to extract `Seconds_behind_master`
  - We strongly recommend using a custom heartbeat mechanism such as `pt-heartbeat`, with subsecond resolution. The sample query above works well with `pt-heartbeat` subsecond timestamps.
  - Strictly speaking, you don't have to provide a replication-lag metric. This could be any query that reports any metric. However you're likely interested in replication lag to start with.
  - Note: the default time unit for replication lag is _seconds_
- `CacheMillis`: optional (default: `0`, disabled), cache `MetricQuery` results. For some queries it make senses to poll aggressively (such is replication lag measurement). For some other queries, it does not. You may, [for example](#non-lag-metrics), throttle on master's load instead of replication lag. Or on master's history length. In such cases you may wish to only query the master in longer intervals. When `CacheMillis > 0` `freno` will cache _valid_ (non-error) query results for specified number of milliseconds.
- `ThrottleThreshold`: an upper limit for valid collected values. If value collected (via `MetricQuery`) is below or equal to `ThrottleThreshold`, cluster is considered to be good to write to. If higher, then cluster writes will need to be throttled.
  - Note: valid range is `[0..)` (`0` or more), where lower values are stricter and higher values are more permissive.
  - Note: use _seconds_ as replication lag time unit. In the above we throttle above `1.0` seconds.
- `IgnoreHostsCount`: number of hosts that can be ignored while aggregating cluster's values. For example, if `IgnoreHostsCount` is `2`, then up to `2` hosts that have errors are silently ignored. Or, if there's no errors, the two highest values will be ignored (so if these two values exceed the cluster's threshold, `freno` may still be happy to allow writes to the cluster).
- `IgnoreHostsThreshold`: applies a conditional to the `IgnoreHostsCount` logic: for hosts with errors, no conditional applies. For hosts reporting a value, the value needs to be _higher_ than given threshold to be ignored.
  A use case: ignore up to `n` lagging replicas, on condition that they're lagging _at least_ `10.0sec`.
- `HttpCheckPort`: when `> 0`, and together with `HttpCheckPath`, `freno` will run a HTTP check on the MySQL boxes. For a given cluster there can only be one HTTP check on a MySQL box, even if one has multiple MySQL services running on that box.
  The HTTP check may return any HTTP status. The `404 Not Found` status is special: `freno` will completely disregard hosts where HTTP checks return `404`.

  You may override `HttpCheckPort` on specific clusters. Set to `-1` to disable HTTP check.
- `HttpCheckPath`: path to test. e.g. when `"HttpCheckPort": 1234` and `"HttpCheckPath": "health"`, `freno` will test `http://<mysql-box>:1234/health`.

  You may override `HttpCheckPath` on specific clusters.
- `IgnoreHosts`: array of substrings. A host is completely ignored by `freno` if it contains a substring listed in `IgnoreHosts`.
  Like other values, this value can be overridden per-cluster. A non-empty `IgnoreHosts` in a specific cluster will replace the `MySQL` scope definition, for that cluster. An empty `IgnoreHosts` in a cluster scope will not un-ignore the patterns specified in `MySQL` scope. If you want to un-ignore the `MySQL` scope use some thing like `"IgnoreHosts": ["--no-such-pattern--"],`, known to never match any of your hosts.
- `FallbackCluster`: optional configured cluster used by `/check` and `/check-read` when the requested MySQL cluster name is not configured. The value must exactly match a key in `Clusters`. Exact configured names always take precedence, including when their metric or threshold is unavailable.

Looking at clusters configuration:

```json
"Clusters": {
  "prod4": {
    "ThrottleThreshold": 0.8,
    "HAProxySettings": {
      "Host": "my.haproxy.mydomain.com",
      "Port": 1001,
      "PoolName": "my_prod4_pool"
    }
  },
  "sharded": {
    "IgnoreHosts": [
      "us-east-2"
    ],
    "VitessSettings": {
      "API": "https://vtctld.example.com/api/",
      "Keyspace": "my_sharded_ks",
      "TabletType": "REPLICA"
    }
  },
  "local": {
    "User": "msandbox",
    "Password": "msandbox",
    "IgnoreHostsCount": 1,
    "StaticHostsSettings" : {
        "Hosts": [
          "127.0.0.1:22293",
          "127.0.0.1:22294",
          "127.0.0.1:22295"
        ]
    }
  }
}
```

This introduces the `prod4`, `sharded`, and `local` clusters. Without `FallbackCluster`, `freno` only serves requests for configured clusters; any other request (e.g. `/check/archive/mysql/prod7`) is answered with `HTTP 404`. With `"FallbackCluster": "prod4"`, ordinary `/check` and `/check-read` requests for unknown MySQL cluster names use `prod4`'s metric, threshold, and throttling state.

Noteworthy:

- `prod4` chooses to (but doesn't have to) override the `ThrottleThreshold` to `0.8` seconds
- `prod4` list of servers is dictated by `HAProxy`. `freno` will routinely and dynamically poll given HAProxy server for list of hosts. These will include any hosts not in `NOLB`.
- `sharded` discovers `REPLICA` tablets by default. Set `TabletType` to `PRIMARY` for a separately configured primary safety metric. `MASTER` is accepted as a deprecated alias and normalized to `PRIMARY`.
- `local` cluster chooses to override `User`, `Password` and `IgnoreHostsCount`.
- `local` cluster defines a static list of hosts.


### Non lag metrics

`freno` isn't necessarily about replication lag. You may choose to use different thresholds appropriate for your setup and workload. For example, you may choose to monitor the master (as opposed of the replicas) and read some metric such as `threads_running`. An example configuration would be:

```json
"Clusters": {
  "master8": {
    "MetricQuery": "show global variables like 'threads_running'",
    "CacheMillis": 500,
    "ThrottleThreshold": 50,
    "User": "msandbox",
    "Password": "msandbox",
    "StaticHostsSettings" : {
        "Hosts": [
          "my.master8.vip.com:3306"
        ]
    }
  }
}
```

`freno` explicitly recognizes `show global ...` statements and reads the result's numeric value.

Otherwise you may provide any query that returns a single row, single numeric column.

### Composing replica, primary, and ProxySQL protection

#### Behavior changes

**Read before enabling dependencies or recovery.** These settings are opt-in, but
they can reject application work as soon as the new configuration takes effect:

- **Startup and leadership recovery:** a cluster with `RecoveryDurationMillis > 0`
  must establish a full continuously healthy window after every process start or
  leadership acquisition, even without `RequiredClusters`. Ordinary write checks
  return `429` while a successfully collected metric is recovering; collection
  errors can return `500` instead. Empty-host behavior is controlled separately by
  `FailOnNoHosts`.
- **Configuration changes require a restart:** the running service does not
  live-reload configuration. Applying changes, including rollback, requires
  restarting the affected process. Metrics that retain a positive
  `RecoveryDurationMillis` must establish a new healthy window.
- **Required dependencies fail closed:** missing runtime metrics or collection
  failures, such as incorrect discovery or missing query privileges, return `500`
  for checks that reach that dependency. An empty host list also fails with `500`
  when the dependency has `FailOnNoHosts: true`. Overload or incomplete recovery
  returns `429`; neither response permits writes.
- **Memcache decisions are affected:** when memcache publishing is configured,
  aggregation deletes a cluster's published key while any required dependency is
  missing, in error, recovering, or above its threshold. Previously cached client
  reads are not synchronously invalidated.
- **Enforcement is not a dry run:** adding a probe and referencing it from
  `RequiredClusters` in the same configuration change can immediately block work
  after restart. First use `ShadowRequiredClusters` to measure prospective
  dependency failures without changing the current decision. Standalone
  collection without a shadow reference does not evaluate the composite graph.
- **Coordinate client wait budgets:** a budget shorter than the remaining
  recovery window can expire before recovery finishes. Agree on safe retry/defer
  behavior; do not bypass checks to avoid a timeout.

At service startup, each cluster with required or shadow dependencies, a positive
recovery duration, or fail-on-no-hosts enabled produces one safety-configuration notice on
standard error. It includes the dependency names, effective recovery and throttle
thresholds, recovery duration, and no-host policy, without credentials or query
text. These notices are visible without `--verbose`; `--quiet` suppresses them.

#### Configuration example

A cluster can require additional cluster metrics to pass before `freno` permits
work. Start with `ShadowRequiredClusters` to evaluate separately sampled
primary-load and ProxySQL-capacity checks without enforcing them:

```json
"Clusters": {
  "prod4": {
    "ShadowRequiredClusters": [
      "prod4-primary",
      "prod4-proxysql"
    ],
    "HAProxySettings": {
      "Host": "my.haproxy.mydomain.com",
      "Port": 1001,
      "PoolName": "my_prod4_pool"
    }
  },
  "prod4-primary": {
    "MetricQuery": "show global status like 'Threads_running'",
    "CacheMillis": 500,
    "ThrottleThreshold": 50,
    "RecoveryThreshold": 35,
    "RecoveryDurationMillis": 5000,
    "FailOnNoHosts": true,
    "StaticHostsSettings": {
      "Hosts": [
        "my.prod4.primary.vip.example.com:3306"
      ]
    }
  },
  "prod4-proxysql": {
    "User": "${proxysql_stats_user}",
    "Password": "${proxysql_stats_password}",
    "MetricQuery": "select connection_pressure_percent from operational_metrics.proxysql_capacity limit 1",
    "CacheMillis": 500,
    "ThrottleThreshold": 80,
    "RecoveryThreshold": 60,
    "RecoveryDurationMillis": 5000,
    "FailOnNoHosts": true,
    "StaticHostsSettings": {
      "Hosts": [
        "my.prod4.proxysql.example.com:6032"
      ]
    }
  }
}
```

The ProxySQL query above is illustrative: deployments must provide a scalar query or operational view whose value increases as usable connection capacity is consumed.

- `RequiredClusters` lists additional configured metrics that must return `HTTP 200`. Missing required runtime metrics fail closed.
- `ShadowRequiredClusters` lists prospective dependencies, evaluated only after
  the real check passes. Their failures do not change the HTTP response, priority
  state, shared-domain health, or memcache publication for the requested cluster.
  Checking a probe directly still applies that probe's ordinary admission rules.
- `FailOnNoHosts` changes the legacy no-host response from `HTTP 200` to `HTTP 500`. Enable it for primary and ProxySQL safety probes, where an empty roster cannot prove that work is safe.
- `RecoveryThreshold` is the lower threshold that begins recovery after a cluster has throttled. It must not exceed `ThrottleThreshold`.
- `RecoveryDurationMillis` is the continuous time the metric must remain at or below `RecoveryThreshold` before checks return `HTTP 200` again. A new error or threshold violation resets the recovery window.
- A new process or leader starts configured recovery metrics in the recovering state. It must observe a complete healthy window before admitting work.
- Metric sampling remains centralized in the `freno` leader and uses the existing metric cache. Application checks read the aggregated decision rather than querying the primary or ProxySQL for every batch.
- Check responses include `MetricName`, identifying the replica, primary, or ProxySQL metric that blocked a composite decision. `recovery.mysql.<cluster>.active` reports whether the recovery latch is active.

Configuration loading rejects missing references in either dependency list,
overlap between the two lists on the same cluster, and cycles across their union.
To promote a dependency, remove it from `ShadowRequiredClusters` and add it to
`RequiredClusters` in the same reviewed configuration change.

#### Shadow counters and limits

Both metric endpoints (`/debug/metrics` and `/debug/vars`) export these counters
as integer values:

| Counter | Meaning |
| --- | --- |
| `shadow.http.mysql.<cluster>.total` | Otherwise-allowed external HTTP checks with an applicable shadow graph |
| `shadow.http.mysql.<cluster>.would_reject` | Checks whose prospective metric dependencies would reject |
| `shadow.http.mysql.<cluster>.dependency.<dependency>.would_reject` | First blocking dependency, in configured traversal order |
| `shadow.http.mysql.<cluster>.status.<status>.would_reject` | Prospective dependency failure status (`429` or `500`) |
| `shadow.background.mysql.<cluster>.*` | Same suffixes for otherwise-healthy leader aggregation cycles |

The HTTP denominator includes both write and read checks. Root-only read
threshold overrides do not weaken shadow dependencies. Internal self-checks are
excluded. Requests already rejected by the root or an enforced dependency do not
increment shadow counters. Unknown names resolved through `FallbackCluster` use
the effective configured cluster name, not the requested name.

The observer includes shadows attached to enforced dependencies and recursively
follows both dependency lists on new targets. Each node is evaluated at most once
per observation, and each observation counts at most one rejection. Already
accepted nodes in the real required closure are not resampled. Dependency and
status breakdowns identify the first failure, not every failing probe.

Shadow evaluation uses cached metric values, configured thresholds, recovery
results, and no-host policy; it does not issue SQL or simulate per-app throttling,
randomized policies, deprioritization, or shared-domain policy on new dependencies.
It is a metric-dependency dry run, not a full prediction of future admission.

Background counters are sampling cycles, **not memcache client reads or
application demand**. They are collected even without memcache publishing.
Dependencies with any error (including legacy fail-open no-host results) are
unsafe for publication, matching the existing memcache path; HTTP shadow checks
retain the dependency's configured `FailOnNoHosts` behavior. Background metrics
are hidden on followers; HTTP counters remain process-local and visible.
The 25 ms aggregation loop can reuse 50 ms SQL samples and longer cached
results, so cycles are not independent samples. Do not combine HTTP and
background totals into one rejection rate.

Compute rates from counter deltas over the same process/window, using
`would_reject / total`. A missing or non-increasing total means no observed
coverage, not a healthy candidate. The root rejection counter is registered at
zero on its first observation; dependency/status counters appear only on
failures. Counters reset on restart and do not replace probe health, discovery,
or coverage validation.

### Rolling out primary safety checks

Deploy a binary containing these settings before deploying configuration that
uses them. Verify the capabilities of the deployed binary rather than assuming
that a configuration change installs new server functionality.

1. Deploy the binary with the existing configuration. Without required
   dependencies, fail-on-no-hosts, or a recovery duration, legacy cluster
   decisions retain their existing behavior.
2. Add separately named primary metrics and reference them from the application's
   normal cluster `ShadowRequiredClusters`, not `RequiredClusters`. Validate writer
   discovery, privileges, probe load, error rates, recovery gauges, and both shadow
   counter scopes. Do not add new probes and enforce them in the same deployment.
3. Review thresholds against recent traffic, known incidents, and per-host/shard
   coverage. Averaged monitoring data is not equivalent to the instantaneous SQL
   samples used by the safety check. Missing data must not be interpreted as zero
   or healthy.
4. In a separate reviewed configuration change, move a primary dependency from
   `ShadowRequiredClusters` to `RequiredClusters` on one cluster whose probes have
   been validated. Exercise overload, probe
   failure, missing hosts, leadership changes, and the HTTP and memcache paths.
   Required probes fail closed; verify callers defer work rather than bypassing
   protection on an error.
5. Expand only after reviewing rejection rates, recovery duration, job backlog,
   application errors, and primary health against agreed success/rollback limits.

Choose recovery durations and client wait budgets for the workload, and
coordinate restarts and leadership transitions with application operators.
Neither a longer timeout nor client-side consecutive checks guarantee recovery
during overload.

Rollback enforcement by moving the primary dependency from the normal cluster's
`RequiredClusters` back to `ShadowRequiredClusters`, keeping its existing replica-lag configuration.
Apply the configuration and restart the affected process; this is not an
instantaneous runtime toggle, and any remaining recovery-enabled metrics need
a new healthy window. Remove probe configurations only after removing every
reference to them.

### Scope boundaries

These settings provide reusable admission checks and recovery behavior. Deployments remain responsible for choosing appropriate metrics and thresholds, configuring their host-discovery sources, monitoring probe health and rejection rates, and controlling application concurrency or fallback behavior.
