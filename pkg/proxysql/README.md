# ProxySQL

This package implements freno store support for [ProxySQL](https://proxysql.com/)

Freno will probe servers found in the `stats.stats_mysql_connection_pool` ProxySQL admin table that have either status:
1. `ONLINE` - connect, ping and replication checks pass
1. `SHUNNED_REPLICATION_LAG` - connect and ping checks pass, replication is lagging

All other statuses _(eg: `SHUNNED` and `OFFLINE_SOFT`)_ are considered unhealthy and therefore are not probed by freno

## Requirements
1. The [ProxySQL monitor module](https://github.com/sysown/proxysql/wiki/Monitor-Module) is enabled, ie: [`mysql-monitor_enabled`](https://github.com/sysown/proxysql/wiki/Global-variables#mysql-monitor_enabled) is `true`
1. The `max_replication_lag` column is defined for backend servers in [the `mysql_servers` admin table](https://github.com/sysown/proxysql/wiki/Main-(runtime)#mysql_servers)
