# MySQL backend


This documents how `freno` achieves high availability and consistent state with a `MySQL` backend. As an alternative, see [Raft](raft.md).


### The setup

You may use any number of `freno` nodes, which will all connect to same MySQL backend. The nodes will pick leader by synching on the MySQL backend, and will not directly communicate with each other.

The following depicts a possible setup to provide with `freno` high availability:

- A highly available MySQL setup: the HA of MySQL is outside `freno`'s scope. Consider [orchestrator](https://github.com/github/orchestrator).
- `2` or more `freno` nodes configured
- Configure all to use the same `BackendMySQLHost`.
- HAProxy in front of `freno` nodes.
  - HAProxy only directs traffic to the _leader_. `freno` has specialized `/leader-check`.
  - Sample HAProxy configuration can be found in [haproxy.cfg](../resources/haproxy.cfg)
- Clients to talk to HAProxy
  - Implicitly, all clients only talk to the _leader_

### Configuration

Let's dissect the general section of the [sample config file](../resources/freno.conf.sample.json):


```json
{
  "BackendMySQLHost": "mysql.example.com",
  "BackendMySQLPort": 3306,
  "BackendMySQLSchema": "freno_backend",
  "BackendMySQLUser": "freno_daemon",
  "BackendMySQLPassword": "123456",
  "BackendMySQLTlsCaCertPath": "/usr/local/share/certs/ca.crt",
  "BackendMySQLTlsClientCertPath": "/usr/local/share/certs/client.crt",
  "BackendMySQLTlsClientKeyPath": "/usr/local/share/certs/client.key",
  "Domain": "us-east-1/production",
  "ShareDomain": "production",
}
```

- `BackendMySQLHost`: MySQL master hostname
- `BackendMySQLPort`: MySQL master port
- `BackendMySQLSchema`: schema where `freno` will read/write state (see below)
- `BackendMySQLUser`: user with read+write privileges on backend schema
- `BackendMySQLTlsCaCertPath´: optional file system path for the PEM-encoded CA certificate to be used when connecting to mysql using TLS
- `BackendMySQLTlsClientCertPath`: optional file system path for the PEM-encoded client certificate
- `BackendMySQLTlsClientKeyPath`:  optional file system path for the PEM-encoded client key
- `BackendMySQLPassword`: password
- `Domain`: the same MySQL backend can serve multiple, unrelated `freno` clusters. Nodes within the same cluster should have the same `Domain` value and will compete for leadership.
- `ShareDomain`: it is possible for clusters to collaborate. Clusters with same `ShareDomain` will consul with each other's metric health reports. A cluster may reject a `check` request if another cluster considers the `check` metrics unhealthy.

You may exchange the above for environment variables:
```json
{
  "BackendMySQLHost": "${MYSQL_BACKEND_HOST}",
  "BackendMySQLPort": 3306,
  "BackendMySQLSchema": "${MYSQL_BACKEND_SCHEMA}",
  "BackendMySQLUser": "${MYSQL_BACKEND_RW_USER}",
  "BackendMySQLPassword": "${MYSQL_BACKEND_RW_PASSWORD}",
  "BackendMySQLTlsCaCertPath": "${MYSQL_BACKEND_CA_CERT}",
  "BackendMySQLTlsClientCertPath": "${MYSQL_BACKEND_CLIENT_CERT}",
  "BackendMySQLTlsClientKeyPath": "${MYSQL_BACKEND_CLIENT_KEY}",
  "Domain": "us-east-1/production",
  "ShareDomain": "production",
}
```

and export such variables to the `freno` daemon.


### MySQL schema

The backend schema should have these tables:

```sql
CREATE TABLE service_election (
  domain varchar(32) NOT NULL,
  share_domain varchar(32) NOT NULL,
  service_id varchar(128) NOT NULL,
  last_seen_active timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (domain),
  KEY share_domain_idx (share_domain,last_seen_active)
);

CREATE TABLE throttled_apps (
  app_name varchar(128) NOT NULL,
	throttled_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL,
	ratio DOUBLE,
  PRIMARY KEY (app_name)
);
```

The `BackendMySQLUser` account must have `SELECT, INSERT, DELETE, UPDATE` privileges on those tables.
