# Memcache

`freno` can be instructed to write aggregated metrics to `memcache`. This allows clients to get data directly from `memcached` without even hitting `freno` HTTP. This, in turn, decouples read load from `freno`.

Instruct `freno` to write aggregated metrics to `memcache` via `MemcacheServers` in the [config file](../resources/freno.conf.sample.json). List all `memcache` servers like so:

```json
{
  "MemcacheServers": [
    "memcache.server.one:11211",
    "memcache.server.two:11211",
    "memcache.server.three:11211"
  ],
}
```

Optionally set `MemcachePath` (default is `"freno"`):
```json
{
  "MemcacheServers": [
    "memcache.server.one:11211",
    "memcache.server.two:11211",
    "memcache.server.three:11211"
  ],
  "MemcachePath": "freno-production",
}
```


### Memcache entries

`freno` will write entries to memcache as follows:

- The key will be of the form `<prefix>/<store-type>/<store-name>`
  - `<prefix>` can be set via the `MemcachePath`, and defaults to `freno`
  - An example key might be `freno/mysql/main1`
- The value will be of the form `<epochmillis>:<aggregated-value>`.
  - As example, it might be `1497418678836:0.54` where `1497418678836` is the unix epoch in milliseconds, and `0.54` is the aggregated value.
  - Embedding the epoch within the value allows the app to double-check the validity of the value, or go into more granular validation.
