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
- `/throttle-app/<app-name>/<ttlMinutes>/ratio`: refuse partial/complete access to an app for a limited amount of time. Examples:

  - `/throttle-app/archive/30/1`: completely refuse `/check/archive/*` requests for a duration of `30` minutes
  - `/throttle-app/archive/30`: same, shorthand
  - `/throttle-app/archive/30/0.9`: _mostly_ refuse `/check/archive/*` requests for a duration of `30` minutes. On average (random dice roll), `9` out of `10` requests (i.e. `90%`) will be denied, and one approved.
  - `/throttle-app/archive/30/0.5`: refuse `50%` of `/check/archive/*` requests for a duration of `30` minutes
  - `/throttle-app/archive/0/0.3`: if already throttled, maintain same TTL and change ratio to `0.3` (`30%` refused). If not already throttled, TTL is one hour
  - `/throttle-app/archive`: completely refuse `/check/archive/*` requests for a duration of 1 hour

- `/unthrottle-app/<app-name>`: remove any imposed throttling constraint from given app. Example:
  `/throttled-apps` will re-allow the `archive` app to get valid response from `/check/archive/*` requests.
  Throttling will of course still consider cluster status, which is never overridden.

- `/throttled-apps`: list currently throttled apps.

### General requests

- `/lb-check`: returns `HTTP 200`. Indicates the node is alive
- `/leader-check`: returns `HTTP 200` when the node is the `raft` leader, or `404` otherwise.
- `/hostname`: node host name

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
