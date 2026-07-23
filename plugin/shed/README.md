# shed

## Name

*shed* - serializes UDP response writes per listener socket and sheds load when the socket cannot keep up.

## Description

UDP responses written back through one listener socket serialize on the Go runtime's internal
fdMutex, which allows at most 2^20-1 concurrent operations (holders plus waiters) per file
descriptor and terminates the process with

~~~ txt
panic: too many concurrent operations on a single file or socket (max 1048575)
~~~

when that is exceeded. CoreDNS serves UDP with one goroutine per query, all writing back through
the shared packet connection, so when queries arrive faster than the socket's serialized writes
drain, every excess in-flight query parks its goroutine in that wait queue and nothing bounds the
pile. Observed in production: ~2.8M goroutines and 60GiB RSS before the panic.

The *shed* plugin makes that panic structurally unreachable, per UDP listener socket:

* **Single writer** - responses are not written by the handler goroutine. The packed response is
  pushed onto a bounded per-socket stack (fixed depth 1024) and one writer goroutine per socket
  performs the wire writes, so the file descriptor never sees more than one writer. The stack
  evicts the oldest entry when full and the writer pops the newest first, so under overload the
  socket's residual capacity always goes to the freshest response. The depth is a fixed burst
  budget (roughly 12-16ms of a typical socket's drain rate), not a tunable.
* **Coupled shedding** - while a socket's stack is full, arriving queries on that socket are
  dropped before any plugin runs; work admitted then would only produce a response destined for
  eviction. There is no configuration: the stack's fullness is the signal.

Drops are silent - no response is written, so the client's resolver retries against another
server, the standard load-shedding contract for UDP DNS. Every drop is counted.

The plugin only acts on UDP; TCP queries pass through untouched. It can only be used in plain DNS
server blocks (not *tls*, *grpc*, *https* or *quic*), which is enforced at startup. It should be
listed before (above) the *prometheus* plugin in the plugin chain, so that shed drops are never
counted as handled requests by the *prometheus* plugin - which is where this plugin sits by
default.

When several server blocks share a listener, any block with *shed* installs the write discipline
for every write on that socket, while the pre-chain shedding only runs in blocks that carry the
directive - keep it uniform across blocks sharing a listener. The discipline covers every response
written through `WriteMsg`, which is how every plugin responds; a plugin writing raw bytes with
`ResponseWriter.Write` would bypass it.

## Syntax

~~~ txt
shed
~~~

The plugin takes no arguments.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric is exported:

* `coredns_shed_dropped_total{server, reason}` - counter of dropped queries and responses. The
  `reason` label is `query` for queries dropped before the plugin chain because the socket's
  stack was full, and `response` for responses dropped at the write boundary (evicted by a newer
  response, failed to reach the wire, or arriving during shutdown).

## Examples

Protect the UDP listener while forwarding:

~~~ corefile
. {
    shed
    forward . 8.8.8.8
}
~~~

## See Also

The fdMutex limit is enforced in `GOROOT/src/internal/poll/fd_mutex.go`.
