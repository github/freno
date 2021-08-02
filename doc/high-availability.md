# High Availability

`freno` offers two alternate methods for high availability:

1. [raft](raft.md) consensus, where `freno` nodes are all `raft` nodes and coordinate state with each other.
2. [MySQL](mysql-backend.md) based, where `freno` uses a `MySQL` backend for state and leadership resolution. In this setup `freno` assumes `MySQL` is available.

MySQL backend adds MySQL as a dependency, and also requires the user to maintain high availability for MySQL itself. It makes sense in environments where MySQL HA is a solved problem.

`raft` has less dependencies, but is also more difficult to deploy on some setups, namely Kubernetes, because of the strict need for nodes to explicitly know each other by name/IP.

### Force leadership

It is possible to instruct a `freno` daemon to assume it is the leader, no matter what consensus says.

- Provide the `--force-leadership` flag.
- This does not affect other nodes. Another node may _also_ believe its the leader, either because of consensus or because of similar configuration.

This flag can be used in emergency cases where consensus cannot be established, due to hardware/network issues.
