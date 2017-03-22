# High Availability

`freno` is a highly available service that uses the `Raft` consensus protocol to coordinate between multiple running nodes. There is a single leader node which probes data from backend stores. When the leader steps down, another takes its place. For the first few seconds it would refuse connections (stepping up as `raft` leader will take a couple seconds) and it will likely have no metrics to share. Within a few seconds it will have all the info it needs to serve.

Events/commands passed to one node are shared via `raft` consensus to other nodes; a newly promoted leader would have all the necessary events to pick up from the place the previous leader stepped down.

### The setup

Sample HAProxy configuration can be found in [haproxy.cfg](../resources/haproxy.cfg)

### Single node mode

It is possible to run `freno` as a single node service. To do that, provide the following in the config file:

```
  "RaftNodes": []
```

i.e. declare no nodes at all. `freno` will still run with `raft` consensus, but will be considered as a standalone node. It will benefit from `raft` event persistence, and dynamic changes will survive a node restart.
