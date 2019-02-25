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

This introduces two clusters: `prod4` and `local`. `freno` will only serve requests for these two clusters. Any other request (e.g. `/check/archive/mysql/prod7`) will be answered with `HTTP 500` -- an unknown metric

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

Otherwise you may provide any query that returns a single row, single numeric column. As another example, consider this query to identify the history length on a master:

```json
"Clusters": {
  "cluster8-writer": {
    "MetricQuery": "SELECT count FROM information_schema.innodb_metrics WHERE name = 'trx_rseg_history_len'",
    "CacheMillis": 1000,
    "ThrottleThreshold": 100000,
    "IgnoreHostsCount": 0,
    "User": "msandbox",
    "Password": "msandbox",
    "HAProxySettings": {
      "Host": "my.haproxy.mydomain.com",
      "Port": 2001,
      "PoolName": "my_prod8_writer"
    }
  }
}
```

### Meta checks

You may also define _meta checks_ that aggregate multiple other checks. With a single API call on a meta-check you will implicitly invoke multiple other checks. A meta-check only responds with `HTTP 200 OK` when all aggregated checks return the same.

Config example:

```json
"Stores": {
  "Meta": {
    "mysql/prod4": [
      "mysql/prod4-lag",
      "mysql/prod4-writer"
    ]
  },
  "MySQL": {
    "Clusters": {
      "prod4-lag": {
        ...
      },
      "prod4-writer": {
        ...
      }
    }
  }
}
```

A call to `api/check/myapp/mysql/prod4` will invoke both `api/check/myapp/mysql/prod4-lag` and `api/check/myapp/mysql/prod4-writer` and return the _worst_ response.

The name of the meta check is flexible. The below is perfectly valid:

```json
"Meta": {
  "foo/bar": [
    "mysql/prod4-lag",
    "mysql/prod4-writer"
  ],
  "backend/database": [
    "mysql/prod1-lag",
    "mysql/prod2-lag",
    "mysql/prod3-lag"
  ]
},
```

You will need to ensure the listed checks do indeed exist in the config file. You may have multiple levels of meta-checks but behavior is unexpected if you create an infinite loop.
