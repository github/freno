# freno

[![build status](https://travis-ci.org/github/freno.svg)](https://travis-ci.org/github/freno) [![downloads](https://img.shields.io/github/downloads/github/freno/total.svg)](https://github.com/github/freno/releases) [![release](https://img.shields.io/github/release/github/freno.svg)](https://github.com/github/freno/releases)

Cooperative, highly available throttler service: clients use `freno` to throttle writes to a resource.

Current implementation can throttle writes to (multiple) MySQL clusters, based on replication status for those clusters. `freno` will throttle cooperative clients when replication lag exceeds a pre-defined threshold.

`freno` dynamically adapts to changes in server inventory; it can further be controlled by the user to force throttling of certain apps.

`freno` is highly available and uses `raft` consensus protocol to decide leadership and to pass user events between member nodes.


### Cooperative

`freno` collects data from backend stores (at this time MySQL only) and has the logic to answer the question "may I write to the backend store?"

Clients (application, scripts, jobs) are expected to consult with `freno`. `freno` is not a proxy between the client and the backend store. It merely observes the store and states "you're good to write" or "you should stop writing". Clients are expected to consult with `freno` and respect its recommendation.

### Stores and apps

`freno` collects data per data store. E.g. when probing MySQL clusters it will collect replication lag per cluster, independently. Backend store metrics are collected automatically and represent absolute truths.

`freno` serves clients, identified as _apps_. Since `freno` is cooperative, it trusts apps to identify themselves. Apps can be managed: `freno` can be instructed to forcibly throttle a certain app. This is so as to enable other, high priority apps to run to completion. `freno` merely accepts instructions on who to throttle, and does not have scheduling/prioritization logic of its own.

### MySQL

`freno` is originally designed to provide a unified, self adapting solution to MySQL throttling: controlling writes while maintaining low replication lag.

`freno` is configured with a pre-defined list of MySQL clusters. This may includes credentials, lag (or other) inspection query, and expected thresholds. For each cluster, `freno` needs to know what servers to probe and collect data from. For each cluster, you may provide this list:

- static, hard coded list of `hostname[:port]`
- dynamic. Hosts may come and go, and throttling may adapt to these changes. Supported dynamic options:
  - via `haproxy`: provide `freno` with a `haproxy` URL and backend/pool name, and `freno` will periodically parse the list of enabled servers in that pool and dynamically adapt to probe it.

Read more about [freno and MySQL throttling](doc/mysql.md)

### Use cases

`freno` is useful for bulk operations: massive loading/archiving tasks, schema migrations, mass updates. Such operations typically walk through thousands to millions of rows and may cause undesired effects such as MySQL replication lags. By breaking these tasks to small subtasks (e.g. `100` rows at a time), and by consulting `freno` before applying each such subtask, we are able to achieve the same result without ill effect to the database and to the application that uses it.

`freno` is not suitable for OLTP queries.

### HTTP

`freno` serves requests via `HTTP`. The most important request is the `check` request: "May this app write to this store?". `freno` appreciates `HEAD` requests (`GET` are also accepted, with more overhead) and responds with status codes:

- `200` (OK): Application may write to data store
- `404` (Not Found): Unknown metric name.
- `417` (Expectation Failed): Requesting application is explicitly forbidden to write.
- `429` (Too Many Requests): Do not write. A normal state indicating the store's state does not meet expected threshold.
- `500` (Internal Server Error): Internal error. Do not write.

Read more on [HTTP requests & responses](doc/http.md)

### Clients

Clients will commonly issue `/check/...` requests via `HEAD`.

Clients can be expected to issue many requests per second. `freno` is lightweight in resources. It should be just fine to hit `freno` hundreds of times per second. It depends on your hardware and resources, of course.

It makes sense to hit `freno` in the whereabouts of the granularity one is looking at. If your client is to throttle on a `1000ms` replication lag, checking `freno` `200` times per sec may be overdoing it. However if you wish to keep your clients naive and without caching this should be fine.

Read more on [clients](doc/clients.md)

### Raft

`freno` uses `raft` to provide high availability. `freno` nodes will compete for _leadership_ and only the leader will collect metrics and should serve clients.

Read more on `raft` and [High Availability](doc/high-availability.md)

### Configuration

See [sample config file](resources/freno.conf.sample.json). Also find:

- [General/raft configuration](doc/high-availability.md#configuration) dissection
- [MySQL-specific configuration](doc/mysql.md#configuration) dissection

### Deployment

See [deployment docs](doc/deploy.md) for suggestions on a recommended `freno` deployment setup.

### Resources

You may find various [resources](resources/) for setting up `freno` in your environment.

### What's in a name?

"Freno" is Spanish for "brake", as in _car brake_. Basically we just wanted to call it "throttler" or "throttled" but both these names are in use by multiple other repositories and we went looking for something else. When we looked up the word "freno" in a dictionary, we found the following sentence:

> Echa el freno, magdaleno!

This reminded us of the 80's and that was it.

### Project status

This project is under active development.

### Contributing

This repository is [open to contributions](.github/CONTRIBUTING.md). Please also see [code of conduct](.github/CODE_OF_CONDUCT.md)

### License

This project is released under the [MIT LICENSE](LICENSE). Please note it includes 3rd party dependencies release under their own licenses; these are found under [vendor](https://github.com/github/freno/tree/master/vendor).

### Authors

Authored by GitHub engineering
