# High Availability

`freno` is a highly available service that uses the `raft` consensus protocol to coordinate between multiple running nodes. There is a single leader node which probes data from backend stores. When the leader steps down, another takes its place. For the first few seconds it would refuse connections (stepping up as `raft` leader will take a couple seconds) and it will likely have no metrics to share. Within a few seconds it will have all the info it needs to serve.

Events/commands passed to one node are shared via `raft` consensus to other nodes; a newly promoted leader would have all the necessary events to pick up from the place the previous leader stepped down.

### The setup

`raft` recommended number of nodes is `3` or `5`. All `freno` nodes are `raft` members.

The following depicts a possible setup to provide with `freno` high availability:

- `3` or `5` (say `n`) `freno` nodes.
- All nodes to have similar config. Config's `"RaftNodes"` to list all `n` nodes (including the local node). Use IP addresses.
  - What we get: `n` nodes talking to each other, one of them becoming _leader_. Only the leader collects data hence is the only one that can actually serve client checks.
- HAProxy in front of `freno` nodes.
  - HAProxy only directs traffic to the _leader_. `freno` has specialized `/leader-check`.
  - Sample HAProxy configuration can be found in [haproxy.cfg](../resources/haproxy.cfg)
- Clients to talk to HAProxy
  - Implicitly, all clients only talk to the _leader_

### Raft

`freno` is a `Go` project. It uses the [Hashicorp](https://github.com/hashicorp/raft) raft implementation, with [Bolt](https://github.com/boltdb/bolt) as backend store.

`freno` stores the `Bolt` data in `freno-raft.db`, under the `RaftDataDir` path as defined in config file.

### Single node mode

It is possible to run `freno` as a single node service (meaning no high availability). To do that, provide the following in the config file:

```
  "RaftNodes": []
```

i.e. declare no nodes at all. `freno` will still run with `raft` consensus, but will be considered as a standalone node. It will benefit from `raft` event persistence, and dynamic changes will survive a node restart.
