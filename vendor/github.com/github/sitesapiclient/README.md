# sites-api client

Importing and using this package can be done with this import statement

```go
import (
    sitesapi "github.com/github/sitesapiclient"
)
```

Creating a new client requires passing in `sitesapi.Config` to the `NewClient` function. The minimum requirement is that `Password` is set. There are sane defaults for the other properties but they can be overridden.

```go
config := sitesapi.Config{Password: $PASSWORD}
client, err := sitesapi.NewClient(http.DefaultClient, &config)
```

Locating an instance can be done with a simple call

```go
resp, err := client.FindInstance(name)
```

Using query parameters is just a `map[string]string` that is passed into the
function being called. If no query parameters pass `nil`. Endpoints that begin with `List` can be passed `params`.

```go
// with params
params := map[string]string{
    "cluster_group": "sauces",
}

resp, err := client.ListCluster(params)

// or without any params
resp, err := client.ListCluster(nil)
```

Here is a full example of how to retrieve clusters

```go
config := sitesapi.Config{Password: password}
client, err := sitesapi.NewClient(http.DefaultClient, &config)
if err != nil {
    fmt.Print(err)
}

resp, err := client.ListClusters(nil)
if err != nil {
    fmt.Print(err)
}

for _, v := range resp {
    fmt.Print(v.Name)
}
```
