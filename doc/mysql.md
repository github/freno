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

### Vitess discovery

For Vitess v22 and v23, configure the native `Vtctld.GetTablets` gRPC service:

```json
"VitessSettings": {
  "GRPCAddress": "vtctld.example.com:15999",
  "GRPCCAFile": "/etc/freno/vitess-ca.pem",
  "GRPCServerName": "vtctld.example.com",
  "Keyspace": "commerce",
  "Shard": "",
  "Cells": ["zone1", "zone2"],
  "TimeoutSecs": 5
}
```

`GRPCAddress` is an explicit `host:port` for a vtctld listener exposing the
`vtctlservice.Vtctld` service, not an HTTP URL or a vttablet endpoint. No port is
inferred from `API`. When `GRPCAddress` is nonempty it takes precedence over
`API`, including on failure: there is no HTTP fallback.

TLS and certificate verification are enabled by default, using system CA roots
and the address's hostname. `GRPCCAFile` optionally replaces the system roots
with certificates from a PEM file. `GRPCServerName` optionally specifies the
expected certificate DNS name, for example when connecting through a different
address. Omit these two fields when system trust and the address's name suffice.
There is no option to skip certificate verification. Servers requiring client
certificates or authentication metadata need additional client support; these
credentials are not configured by this discovery path.

Only for an intentionally plaintext gRPC listener, set `"GRPCInsecure": true`.
This disables transport encryption, not just certificate verification, and
cannot be combined with `GRPCCAFile` or `GRPCServerName`. Do not use it as a
workaround for TLS errors or to migrate an HTTPS configuration to plaintext.

`Keyspace` is required. An empty or omitted `Shard` discovers all shards in that
keyspace. `Cells` restricts discovery to those cells; empty entries are ignored.
If `Cells` is omitted or empty, it inherits `Stores.MySQL.VitessCells`. If no
nonempty cells remain, all cells are eligible. Only `REPLICA` tablets are used,
and their `mysql_hostname` and `mysql_port` supply the probe addresses, not their
vttablet hostname or service ports. The gRPC response has no realtime tablet
health stats; Freno continues to collect its configured MySQL metrics and HTTP
checks.

`TimeoutSecs` bounds each discovery call, including connection establishment;
zero or omitted means five seconds. Each gRPC call owns and closes its
connection. Requests set `Strict=true` so a topology/cell failure is an error,
not a successful partial inventory. Errors are logged and do not replace the
previous inventory. Successful responses with no replicas remain an empty
inventory, as with HTTP discovery.

Existing `API`-only configurations retain the legacy HTTP discovery path and
its realtime-stat filtering. That endpoint was
[removed in Vitess v23.0.7](https://github.com/vitessio/vitess/releases/tag/v23.0.7#legacy-vtctld-http-api-removed);
configure gRPC before upgrading to a version without it. The HTTP transport is
not made compatible with its removal.

The client uses the stable [Vitess v0.21.0 generated schema](https://github.com/vitessio/vitess/blob/v21.0.0/proto/vtctldata.proto) rather than
upgrading to v23 libraries. Its request fields and response match
the public [v22 GetTablets schema](https://github.com/vitessio/vitess/blob/v22.0.0/proto/vtctldata.proto)
and [v23 GetTablets schema](https://github.com/vitessio/vitess/blob/v23.0.8/proto/vtctldata.proto);
the request filters by replica type and results are also filtered locally.
The v0.12.6 schema was also evaluated, but its
[runtime helpers](https://github.com/vitessio/vitess/blob/v12.0.6/go/hack/runtime.go) do not link with
the required modern Go toolchain. Local gRPC-server tests exercise the RPC
contract, filtering, TLS and errors; they are not real v22/v23 vtctld daemon
integration tests.

Building requires Go 1.25 or newer. This is required by the patched gRPC
transport, not by the older Vitess schema; see the upstream
[HTTP/2 memory-exhaustion advisory](https://github.com/advisories/GHSA-vp52-pcj8-j9qc).
Downstream builders must update their Go toolchain and build the new source
revision. Runtime rollout also requires a reachable gRPC listener, authorization
to call `GetTablets`, and the appropriate CA certificates. Installing a new
binary alone does not change `API`-only configurations.


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
