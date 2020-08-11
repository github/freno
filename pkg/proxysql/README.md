# ProxySQL

This package implements freno store support for [ProxySQL](https://proxysql.com/)

## Requirements
1. The [ProxySQL monitor module](https://github.com/sysown/proxysql/wiki/Monitor-Module) is enabled, ie: [`mysql-monitor_enabled`](https://github.com/sysown/proxysql/wiki/Global-variables#mysql-monitor_enabled) is `true`
1. The `max_replication_lag` column is defined for backend servers in [the `mysql_servers` admin table](https://github.com/sysown/proxysql/wiki/Main-(runtime)#mysql_servers)
