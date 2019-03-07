# Log [![GoDoc](https://godoc.org/github.com/go-log/log?status.svg)](https://godoc.org/github.com/go-log/log)

Log is a logging interface for Go. That's it. Pass around the interface.

## Rationale

Users want to standardise logging. Sometimes libraries log. We leave the underlying logging implementation to the user 
while allowing libraries to log by simply expecting something that satisfies the `Logger` interface. This leaves
the user free to pre-configure structure, output, etc.

## Interface

The interface is minimalistic on purpose:

```go
type Logger interface {
    Log(v ...interface{})
    Logf(format string, v ...interface{})
}
```

For more motivation for this minimal interface, see [Dave Cheney's blog post][cheney].

## Implementations

Libraries will only need [the `Logger` interface](#interface), although they may choose to use [the `nest` package][nest] to create subloggers with additional context.

Calling code will need to create a `Logger` interface, and there are a number of implementations and wrappers available to make that easy:

* [capture][] is an implementation that saves logged lines in memory.
    It is especially useful for unit tests that want to check for logged messages.
* [fmt][] is an implementation wrapping [an `io.Writer`][io.Writer] like [`os.Stdout`][os.Stdout].
    It uses [`fmt.Sprint`][fmt.Sprint] and [`Sprintf`][fmt.Sprintf] to generate the logged lines.
* [info][] is an implementation wrapping `Info` and `Infof` calls.
    It can be used to wrap implementations like [`glog.Verbose`][glog.Verbose] and [`logrus.Entry`][logrus.Entry.Info].
* [print][] is an implementation wrapping `Print` and `Printf` calls.
    It can be used to wrap implementations like [`glog.Verbose`][logrus.Entry.Print].
* [log][] is an implementation wrapping [`log.Print`][log.Print] and [`log.Printf`][log.Printf].

Outside of this repository, there are additional wrappers for:

* [appengine/log][appengine], [here][appengine-wrapper].
* [logrus][], [here][logrus-wrapper].
    Although as mentioned above, you can also use the [info][] and [print][] wrappers for logrus.

The `Logger` interface is also simple enough to make writing your own implementation or wrapper very straightforward.

## Example

Pre-configure a logger using [`WithFields`][logrus.WithFields] and pass it as an option to a library:

```go
import (
	"github.com/go-log/log/print"
	"github.com/lib/foo"
	"github.com/sirupsen/logrus"
)

logger := print.New(logrus.WithFields(logrus.Fields{
	"library": "github.com/lib/foo",
}))

f := foo.New(logger)
```

## Related projects

[github.com/go-logr/logr][logr] is a similar interface approach to logging, although [the `logr.Logger` interface][logr.Logger] is more elaborate.

[appengine]: https://cloud.google.com/appengine/docs/standard/go/logs/
[appengine-wrapper]: https://github.com/go-log/appengine
[capture]: https://godoc.org/github.com/go-log/log/capture
[cheney]: https://dave.cheney.net/2015/11/05/lets-talk-about-logging
[fmt]: https://godoc.org/github.com/go-log/log/fmt
[fmt.Sprint]: https://golang.org/pkg/fmt/#Sprint
[fmt.Sprintf]: https://golang.org/pkg/fmt/#Sprintf
[glog.Verbose]: https://godoc.org/github.com/golang/glog#Verbose.Info
[info]: https://godoc.org/github.com/go-log/log/info
[io.Writer]: https://golang.org/pkg/io/#Writer
[log]: https://godoc.org/github.com/go-log/log/log
[log.Print]: https://golang.org/pkg/log/#Print
[log.Printf]: https://golang.org/pkg/log/#Printf
[logr]: https://github.com/go-logr/logr
[logr.Logger]: https://godoc.org/github.com/go-logr/logr#Logger
[logrus]: https://github.com/sirupsen/logrus
[logrus-wrapper]: https://github.com/go-log/logrus
[logrus.Entry.Info]: https://godoc.org/github.com/sirupsen/logrus#Entry.Info
[logrus.Entry.Print]: https://godoc.org/github.com/sirupsen/logrus#Entry.Print
[logrus.WithFields]: https://godoc.org/github.com/sirupsen/logrus#WithFields
[nest]: https://godoc.org/github.com/go-log/log/nest
[os.Stdout]: https://golang.org/pkg/os/#Stdout
[print]: https://godoc.org/github.com/go-log/log/print
