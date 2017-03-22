# Compared to Doorman

`freno` is both similar to and different from [Doorman](https://github.com/youtube/doorman).

### Capacity vs. State

`freno` does not discuss _capacity_. It does not know what the capacity is for the backend store (e.g. database/replication write capacity).

`freno` clients do not request for a specific capacity.

Instead, clients are greedy: they have a task to do; they're willing to break it to small subtasks and they're willing to throttle those subtasks based on `freno`'s evaluation of _the state of the backend store_.

`freno` observes the backend stores and compares collected metrics with pre-configured thresholds. If a backend (e.g. MySQL replication cluster) satisfies the threshold, `freno` is generally happy and recommends clients to proceed. If metrics do not satisfy the threshold it recommends clients to stop.

This behavior is in particular useful with MySQL based applications, where parts of the application, that do not throttle, may burst in at any time and be given high priority. The `freno` clients must always adapt to the current state and give way as needed.

As contrast, `Doorman` manages a resource's write capacity by dividing the known capacity between clients, where each client dictates up front its own expected write volume.

### Lease

`freno` does not impose a lease length. A client that gets approval from `freno` is expected to run a _small enough_ task.

### Cooperation

Both `freno` and `Doorman` are cooperative. The clients are _expected_ to talk to these services and are _expected_ to adhere to their recommendations.

### Availability

`freno` is a highly available service that uses the `raft` consensus protocol to coordinate between multiple running nodes. There is a single leader node which probes data from backend stores. When the leader steps down, another takes its place. For the first few seconds it would refuse connections (stepping up as `raft` leader will take a couple seconds) and it will likely have no metrics to share. Within a few seconds it will have all the info it needs to serve.

Events/commands passed to one node are shared via `raft` consensus to other nodes; a newly promoted leader would have all the necessary events to pick up from the place the previous leader stepped down.

Should the entire `freno` be unavailable, the clients are expected to throttle. Clients are only allowed writes when `freno` is up and running and reports `HTTP 200`.
