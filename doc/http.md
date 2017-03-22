# HTTP

`freno` serves requests via `HTTP`. Requests/responses are short enough that `HTTP` does not incur a substantial overhead.

Client/automated requests should use `HEAD` requests, and manual/human requests may use `GET` requests. Both variations return the same HTTP status codes.

# 'check' requests

The `check` request is the one important question `freno` must answer: "may this app write to this datastore?"

For example in `/check/archive/mysql/main1` the `archive` app wishes to write to the `main1` MySQL cluster.

`freno` answers by choosing an appropriate HTTP status code, as follows:

# Status codes

- `200` (OK): Application may write to data store
- `417` (Expectation Failed): Requesting application is explicitly forbidden to write.
- `429` (Too Many Requests): Do not write. A normal state indicating the store's state does not meet expected threshold.
- `500` (Internal Server Error): Internal error. Do not write.

Notes:

- Clients should only proceed to write on status code `200`.
- `417` (Expectation Failed) results from a user/admin telling `freno` to reject requests from certain apps
- `429` (Too Many Requests) is just a normal "do not write" response, and is a frequent response if the store is busy.
- `500` (Internal Server Error) can happen if the node just started, or otherwise `freno` met an unexpected error. Try a `GET` (more informative) request or search the logs.

# Other requests

`freno` supports the following:

### Client requests

- `/check/<app>/<store-type>/<store-name>`: the most important request: may `app` write to a backend store?

  - `<app>` can be any name, does not need to be pre-defined
  - `mysql` is the only supported `<store-type>` at this time
  - `<store-name>` must be defined in the configuration file
  - Example: `/check/archive/mysql/main1`

- `/throttle-app/<app-name>`: instructs `freno` to deny writes to the `archive` app. `/check/archive/...` requests will be responded by `417` (Expectation Failed).

   Example: `/throttle-app/archive`

- `/unthrottle-app/<app-name>`: Undoes a `/throttle-app` request. App will be able to proceed as normal, based on store status.

### General requests

- `/lb-check`: returns `HTTP 200`. Indicates the node is alive
- `/leader-check`: returns `HTTP 200` when the node is the `raft` leader, or `404` otherwise.
- `/hostname`: node host name

# GET method

`GET` and `HEAD` respond with same status codes. But `GET` requests compute and return additional data. Automated requests should not be interested in this data; the status code is what should guide the clients. However humans or manual requests may benefit from extra information supplied by the `GET` request.

For example:

A `GET` request for `http://my.freno.service:9777/check/archive/mysql/main-cluster` may yield with:

```json
{
    "StatusCode": 200,
    "Message": "",
    "Value": 0.430933,
    "Threshold": 1
}
```

Extra info such as the threshold or actual replication lag value is irrelevant for automated requests, which should just know whether they're allowed to proceed or not. For humans this is beneficial input.
