# MySQL

`freno` was written to assist in controlling writes to `MySQL` clusters.

### Background

`MySQL` installments typically include a master an multiple replicas. Aggressive write to the master may cause increased replication lags. Replication lags have multiple undesired effects, such as stale replica data.

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
  "ThrottleThreshold": 1.0,
  "Clusters": {
    "..."
  }
}
```

These params apply in general to all MySQL clusters, unless specified differently (overridden) on a per-cluster basis.

- `User`, `Password`: these can be specified as plaintext, or in a `${some_env_variable}` format, in which case `freno` will look up its environment for specified variable. (e.g. a `shell` script invoking `freno` can `export some_env_variable=flyingcircus`)
- `MetricQuery`:
  - if not provided, `freno` will assume you're interested in replication lag, and will issue a `SHOW SLAVE STATUS` to extract `Seconds_behind_master`
  - We strongly recommend using a custom heartbeat mechanism such as `pt-heartbeat`, with subsecond resolution. The sample query above works well with `pt-heartbeat` subsecond timestamps.
  - Strictly speaking, you don't have to provide a replication-lag metric. This could be any query that reports any metric. However you're likely interested in replication lag to start with.
- `ThrottleThreshold`: an upper limit for valid collected values. If value collected (via `MetricQuery`) is below or equal to `ThrottleThreshold`, cluster is considered to be good to write to. If higher, then cluster writes will need to be throttled.

Looking at per-cluster
