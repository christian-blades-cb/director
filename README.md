# Director
URL redirector with a BoltDB backend

# Usage
```shell
$ go get github.com/christian-blades-cb/director
$ director
```

Director is now running on port 8000. Any URL stems that are registered with it will redirect the browser to the destination URL.

## Administration

The admin interface is available on port 8888.

### Registering stems

```shell
$ curl -XPUT http://localhost:8888/stems/iloveurlredirections -d"http://github.com/christian-blades-cb"
```

Now calls to `http://localhost:8000/iloveurlredirections` will redirect to an amazing github user.

### Backups

```shell
$ curl http://localhost:8888/stems > stems.json
```

The output is in the form:

```json
{ 
  "stem1": "destination1",
  "stem2": "destination2"
}
```
