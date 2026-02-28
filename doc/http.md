# HTTP

`freno` serves requests via `HTTP`. Requests/responses are short enough that `HTTP` does not incur a substantial overhead. `freno` listens on configuration's `"ListenPort"`.

Client/automated requests should use `HEAD` requests, and manual/human requests may use `GET` requests. Both variations return the same HTTP status codes.

# 'check' requests

The `check` request is the one important question `freno` must answer: "may this app write to this datastore?"

For example in `/check/archive/mysql/main1` the `archive` app wishes to write to the `main1` MySQL cluster.

`freno` answers by choosing an appropriate HTTP status code, as follows:

# Status codes

- `200` (OK): Application may write to data store
- `404` (Not Found): Unknown metric name.
- `417` (Expectation Failed): Requesting application is explicitly forbidden to write.
- `429` (Too Many Requests): Do not write. A normal state indicating the store's state does not meet expected threshold.
- `500` (Internal Server Error): Internal error. Do not write.

Notes:

- Clients should only proceed to write on status code `200`.
- `404` (Not Found) can be seen when metric name is incorrect, undefined, or if the server is not the leader or was _just_ promoted and didn't get the chance to collect data yet.
- `417` (Expectation Failed) results from a user/admin telling `freno` to reject requests from certain apps
- `429` (Too Many Requests) is just a normal "do not write" response, and is a frequent response if the store is busy.
- `500` (Internal Server Error) can happen if the node just started, or otherwise `freno` met an unexpected error. Try a `GET` (more informative) request or search the logs.

# API

`freno` supports the following:

### Client requests

- `/check/<app>/<store-type>/<store-name>`: the most important request: may `app` write to a backend store?

  - `<app>` can be any name, does not need to be pre-defined
  - `mysql` is the only supported `<store-type>` at this time
  - `<store-name>` must be defined in the configuration file
  - Example: `/check/archive/mysql/main1`

### Control requests

##### Throttle
- `/throttle-app/<app-name>/ttl/<ttlMinutes>/ratio/<ratio>`: refuse partial/complete access to an app for a limited amount of time. Examples:

  - `/throttle-app/archive/ttl/30/ratio/1`: completely refuse `/check/archive/*` requests for a duration of `30` minutes
  - `/throttle-app/archive/ttl/30/ratio/0.9`: _mostly_ refuse `/check/archive/*` requests for a duration of `30` minutes. On average (random dice roll), `9` out of `10` requests (i.e. `90%`) will be denied, and one approved.
  - `/throttle-app/archive/ttl/30/ratio/0.5`: refuse `50%` of `/check/archive/*` requests for a duration of `30` minutes
  
- `/throttle-app/<app-name>/ttl/<ttlMinutes>`:

  - If app is already throttled, modify TTL portion only, without changing the ratio.
  - If app is not already throttled, fully throttle for a duration of `1` hour (`ratio` is implicitly `1`).


- `/throttle-app/<app-name>/ratio/<ratio>`:

  - If app is already throttled, modify ratio portion only, without changing the TTL.
  - If app is not already throttled, throttle with given ratio, for a duration of `1` hour.

- `/throttle-app/<app-name>`: refuse access to an app for `1` hour.

  Same as calling `/throttle-app/<app-name>/ttl/60/ratio/1`. Provided as convenience endpoint.

- `/throttle-app` can take a query parameter `store_name` to throttle the app only on one store (i.e. MySQL cluster). For example `/throttle-app/archive?store_name=mycluster` refuses `/check/archive/mysql/mycluster` requests for `1` hour.


  `/unthrottle-app/archive` will re-allow the `archive` app to get valid response from `/check/archive/*` requests.

  Throttling will of course still consider cluster status, which is never overridden.

- `/throttled-apps`: list currently throttled apps.

##### Usage

- `/recent-apps/<lastMinutes>`: list app/host that have `/check`ed `freno` in the past given minutes. Example:

  - `/recent-apps/30` show which apps from which hosts have issued `check` requests in the past `30` minutes

- `/recent-apps`: no time limit; `freno` keeps up to `24h` of `check` requests.

### General requests

- `/lb-check`: returns `HTTP 200`. Indicates the node is alive
- `/leader-check`: returns `HTTP 200` when the node is the `raft` leader, or `404` otherwise.
- `/hostname`: node host name

### Specialized requests

- `/check-read/<app>/<store-type>/<store-name>/<threshold>`: a specialized check to see whether current value is lower than given threshold.

  As an example, consider `/check-read/archive/mysql/main1/2.5`. This checks whether the current `mysql/main1` store's value is smaller than or equals to `2.5`. The store's configured threshold value is ignored and not tested in this check.

  This read-check _should not be used to approve writes_. Writes should only be approved by using the `/check` request.

  However this check is known to be useful, at least in one common scenario: a monitoring of a MySQL cluster based on replication lag. In such case, we may have write requests followed by read requests. We may happen to know the elapsed time between write & read. As an example, say `2.5s` have passed between the write and read. The check `/check-read/archive/mysql/main1/2.5` confirms or denies that relevant replicas are up-to-date for the `2.5s` elapsed time. We can therefore read from the replicas and safely expect to find the data we wrote `2.5s` ago on the master.

- `/check-if-exists/<app>/<store-type>/<store-name>`: like `/check`, but if the metric is unknown (e.g. `<store-name>` not in `freno`'s configuration), return `200 OK`. This is useful for hybrid systems where some metrics need to be strictly controlled, and some not. `freno` would probe the important stores, and still can serve requests for all stores.

- `/check-read-if-exists/<app>/<store-type>/<store-name>/<threshold>`: like `/check-read`, but if the metric is unknown (e.g. `<store-name>` not in `freno`'s configuration), return `200 OK`. This is useful for hybrid systems where some metrics need to be strictly controlled, and some not. `freno` would probe the important stores, and still can serve requests for all stores.

- `/skip-host/<hostname>/ttl/<ttl-minutes>`: skip host when aggregating metrics for specified number of minutes. If host is already skipped, update the TTL.
- `/skip-host/<hostname>`: same as `/skip-host/<hostname>/ttl/60`
- `/recover-host/<hostname>`: recover a previously skipped host.
- `/skipped-hosts`:  list currently skipped hosts

### Other requests

- `/help`: show all supported request paths

- `/config/memcache`: show the [memcache](memcache.md) configuration used, so freno clients can use it to implement more efficient read strategies.

# GET method

`GET` and `HEAD` respond with same status codes. But `GET` requests compute and return additional data. Automated requests should not be interested in this data; the status code is what should guide the clients. However humans or manual requests may benefit from extra information supplied by the `GET` request.

For example:

A `GET` request for `http://my.freno.service:9777/check/archive/mysql/main1` may yield with:

```json
{
    "StatusCode": 200,
    "Message": "",
    "Value": 0.430933,
    "Threshold": 1
}
```

Extra info such as the threshold or actual replication lag value is irrelevant for automated requests, which should just know whether they're allowed to proceed or not. For humans this is beneficial input.
