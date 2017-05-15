# Clients


Clients will commonly issue `/check/...` requests via `HEAD`. `freno` responds by setting an appropriate [HTTP status code](https://github.com/github/freno/blob/master/doc/http.md#status-codes).

While `GET` method is also supported, the response for `HEAD` requests is shorter and involves less computation by `freno`. `GET` is more useful to humans.

Clients can be expected to issue many requests per second. `freno` is lightweight in resources. It should be just fine to hit `freno` hundreds of times per second. It depends on your hardware and resources, of course.

`freno` probes backend stores continuously and independently of client requests. Client requests merely pick up on the latest metrics collected by `freno`, and do not synchronously wait on servers to be polled.

It makes sense to hit `freno` in the whereabouts of the granularity one is looking at. If your client is to throttle on a `1000ms` replication lag, checking `freno` `200` times per sec may be overdoing it. However if you wish to keep your clients naive and without caching this should be fine.

# Usage samples


### pt-archiver

[pt-archiver](https://www.percona.com/doc/percona-toolkit/2.2/pt-archiver.html) is probably the most popular tool for archiving table data. `pt-archiver` can use `freno` with a plugin. A plugin is available on [FrenoThrottler.pm](../resources/pt-archiver/FrenoThrottler.pm). To make it usable, you will need to:

- Let the plugin know where to find `freno` and which cluster to use (see sample code in comment in plugin file)
- Deploy the plugin. Sample `puppet` deployment would look something like:
  ```
  file { '/usr/share/perl5/FrenoThrottler.pm':
      ensure => file,
      owner  => 'root',
      group  => 'root',
      mode   => '0755',
      source => 'puppet:///modules/percona/usr/share/perl5/FrenoThrottler.pm';
  }
  ```

  This assumes `/usr/share/perl5` is in your `@INC` path (run `perl -e 'print "@INC"'` to confirm).

### shell

```shell
if curl -s -I http://my.freno.com:9777/check/myscript/mysql/main7 | grep -q "200 OK" ; then
  echo "Good to go, do some writes"
else
  echo "Need to throttle; please refrain from writes"
fi
```

### Go

```go
import "net/http"

const frenoUrl = "http://my.freno.com:9777/check/my-go-app/mysql/main7"

func CheckFreno() (canWrite bool, err error) {
	resp, err := http.Head(frenoUrl)
	if err != nil {
		return false, err
	}
	return resp.StatusCode == http.StatusOK, nil
}
```
