# ProxySQL

This package implements freno store support for [ProxySQL](https://proxysql.com/)

## Logic

Freno will probe servers found in the `stats.stats_mysql_connection_pool` ProxySQL admin table that have either status:
1. `ONLINE` - connect, ping and replication checks pass
1. `SHUNNED_REPLICATION_LAG` - connect and ping checks pass, but replication is lagging

All other statuses are considered unhealthy and therefore are ignored by freno, eg:
1. `SHUNNED` - proxysql connot connect and/or ping a backend
1. `OFFLINE_SOFT` - a server that is draining, usually for maintenance, etc
1. `OFFLINE_HARD` - a server that is completely offline

## Requirements
1. The ProxySQL `--no-monitor` flag is not set
1. The [ProxySQL monitor module](https://github.com/sysown/proxysql/wiki/Monitor-Module) is enabled, eg: [`mysql-monitor_enabled`](https://github.com/sysown/proxysql/wiki/Global-variables#mysql-monitor_enabled) is `true`
1. The `max_replication_lag` column is defined for backend servers in [the `mysql_servers` admin table](https://github.com/sysown/proxysql/wiki/Main-(runtime)#mysql_servers)
    - This ensures servers with lag do not receive reads but are still probed by freno

