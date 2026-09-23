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
      "Keyspace": "my_sharded_ks"
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

A cluster can require additional cluster metrics to pass before `freno` permits work. This allows the normal replica-lag check to depend on separately sampled primary-load and ProxySQL-capacity checks:

```json
"Clusters": {
  "prod4": {
    "RequiredClusters": [
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
- `FailOnNoHosts` changes the legacy no-host response from `HTTP 200` to `HTTP 500`. Enable it for primary and ProxySQL safety probes, where an empty roster cannot prove that work is safe.
- `RecoveryThreshold` is the lower threshold that begins recovery after a cluster has throttled. It must not exceed `ThrottleThreshold`.
- `RecoveryDurationMillis` is the continuous time the metric must remain at or below `RecoveryThreshold` before checks return `HTTP 200` again. A new error or threshold violation resets the recovery window.
- A new process or leader starts configured recovery metrics in the recovering state. It must observe a complete healthy window before admitting work.
- Metric sampling remains centralized in the `freno` leader and uses the existing metric cache. Application checks read the aggregated decision rather than querying the primary or ProxySQL for every batch.
- Check responses include `MetricName`, identifying the replica, primary, or ProxySQL metric that blocked a composite decision. `recovery.mysql.<cluster>.active` reports whether the recovery latch is active.

Configuration loading rejects missing `RequiredClusters` references and dependency cycles.

### A1 repair mapping

| A1 repair item | Freno implementation in this change | Remaining work |
| --- | --- | --- |
| Default transitions to one worker | Not a Freno responsibility. | Enforce in the transitions framework. |
| Audit custom worker configurations | Not a Freno responsibility. | Audit and constrain custom transitions in the application. |
| Identify Freno traffic as transitions | Existing Freno API already accepts the application name and emits per-app/per-store counters. | Complete caller rollout with `app=transitions` and monitor that identity. |
| Mark transition writes as low priority | Existing `p=low` behavior remains compatible with composite checks. | Complete application rollout. |
| Page on primary and ProxySQL connection failures | `FailOnNoHosts` and missing-required-metric handling prevent an unobservable probe failure from allowing work. | Add Datadog paging for probe and connectivity failures. |
| Page on sustained primary `Threads_running` | A cached primary metric can now be required by the replica-lag decision; recovery hysteresis prevents immediate release. | Configure cluster thresholds and add the sustained Datadog monitor. |
| Page before connection exhaustion | A cached ProxySQL capacity metric can now be required by the admission decision. | Define the production capacity query, reserve threshold, and Datadog monitor. |
| Detect rising concurrency with falling completions | A derived scalar metric can be configured as another required cluster check. | Define and validate the correlation query and alert thresholds. |
| Alert on sustained Freno rejection | Composite safety failures return normal Freno rejection/error status and existing counters remain available by app and store. | Add the rejection-ratio and minimum-volume Datadog monitor grouped by cluster and `app=transitions`. |
| Provide a cluster-wide emergency pause | Existing app/store throttling remains the emergency control and is evaluated for each required metric. | Operationalize a deterministic transitions-plus-cluster command and runbook. |

These changes provide admission enforcement; they do not create Datadog monitors, change transition worker defaults, or bound replica-to-primary fallback inside the application.
