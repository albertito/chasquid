
# Monitoring

chasquid includes an HTTP server for monitoring purposes, which for security
it is not enabled by default.

You can use the `monitoring_address` configuration option to enable it.

Then just browse the address and human-friendly links to various monitoring
and debugging tools should appear.

These include:

- Command-line flags.
- [Traces](https://pkg.go.dev/blitiri.com.ar/go/chasquid/internal/trace) of
  both short and long lived requests.
- State of the queue.
- State of goroutines.
- [Exported variables](#variables) for whitebox monitoring.
- Profiling endpoints, for use with `go tool pprof` or similar tools.


## Variables

chasquid exports some variables for monitoring, via the standard
[expvar](https://golang.org/pkg/expvar/) package and the
[OpenMetrics](https://openmetrics.io/) text format, which can be useful for
whitebox monitoring.

They're accessible on the monitoring HTTP server, at `/debug/vars` (default
endpoint for expvars) and `/metrics` (common endpoint for openmetrics).

<a name="prometheus"></a>
The `/metrics` endpoint is also compatible with
[Prometheus](https://prometheus.io/).

*Note these are still subject to change, although breaking changes will be
avoided whenever possible, and will be noted in the
[release notes](relnotes.md).*

List of exported variables:

- **chasquid/aliases/hookResults** (hook result -> counter)  
  count of aliases hook results, by hook and result.
- **chasquid/queue/deliverAttempts** (recipient type -> counter)  
  attempts to deliver mail, by recipient type (pipe/local email/remote email).
- **chasquid/queue/dsnQueued** (counter)  
  count of DSNs that we generated (queued).
- **chasquid/queue/itemsWritten** (counter)  
  count of items the queue wrote to disk.
- **chasquid/queue/putCount** (counter)  
  number of envelopes put in the queue.
- **chasquid/smtpIn/commandCount** (map of command -> count)  
  count of SMTP commands received, by command. Note that for unknown commands
  we use `unknown<COMMAND>`.
- **chasquid/smtpIn/dkimSignErrors** (counter)  
  count of DKIM sign errors
- **chasquid/smtpIn/dkimSigned** (counter)  
  count of successful DKIM signs
- **chasquid/smtpIn/dkimVerifyErrors** (counter)  
  count of DKIM verification errors
- **chasquid/smtpIn/dkimVerifyFound** (counter)  
  count of messages with at least one DKIM signature
- **chasquid/smtpIn/dkimVerifyNotFound** (counter)  
  count of messages with no DKIM signatures
- **chasquid/smtpIn/dkimVerifyValid** (counter)  
  count of messages with at least one valid DKIM signature
- **chasquid/smtpIn/hookResults** (result -> counter)  
  count of hook invocations, by result.
- **chasquid/smtpIn/loopsDetected** (counter)  
  count of email loops detected.
- **chasquid/smtpIn/responseCodeCount** (code -> counter)  
  count of response codes returned to incoming SMTP connections, by result
  code.
- **chasquid/smtpIn/securityLevelChecks** (result -> counter)  
  count of security level checks on incoming connections, by result.
- **chasquid/smtpIn/spfResultCount** (result -> counter)  
  count of SPF checks, by result.
- **chasquid/smtpIn/tlsCount** (tls status -> counter)  
  count of TLS statuses (plain/tls) for incoming SMTP connections.
- **chasquid/smtpIn/wrongProtoCount** (command -> counter)  
  count of commands for other protocols (e.g. HTTP commands).
- **chasquid/smtpOut/securityLevelChecks** (result -> counter)  
  count of security level checks on outgoing connections, by result.
- **chasquid/smtpOut/sts/mode** (mode -> counter)  
  count of STS checks on outgoing connections, by mode (enforce/testing).
- **chasquid/smtpOut/sts/security** (result -> counter)  
  count of STS security checks on outgoing connections, by result (pass/fail).
- **chasquid/smtpOut/tlsCount** (status -> counter)  
  count of TLS status (insecure TLS/secure TLS/plain) on outgoing connections.
- **chasquid/sourceDateStr** (string)  
  timestamp when the binary was built, in human readable format.
- **chasquid/sourceDateTimestamp** (int)  
  timestamp when the binary was built, in seconds since epoch.
- **chasquid/sts/cache/expired** (counter)  
  count of expired entries in the STS cache.
- **chasquid/sts/cache/failedFetch** (counter)  
  count of failed fetches in the STS cache.
- **chasquid/sts/cache/fetches** (counter)  
  count of total fetches in the STS cache.
- **chasquid/sts/cache/hits** (counter)  
  count of hits in the STS cache.
- **chasquid/sts/cache/invalid** (counter)  
  count of invalid policies in the STS cache.
- **chasquid/sts/cache/ioErrors** (counter)  
  count of I/O errors when maintaining the STS cache.
- **chasquid/sts/cache/marshalErrors** (counter)  
  count of marshaling errors when maintaining the STS cache.
- **chasquid/sts/cache/refreshCycles** (counter)  
  count of STS cache refresh cycles.
- **chasquid/sts/cache/refreshErrors** (counter)  
  count of STS cache refresh errors.
- **chasquid/sts/cache/refreshes** (counter)  
  count of STS cache refreshes.
- **chasquid/sts/cache/unmarshalErrors** (counter)  
  count of unmarshaling errors in the STS cache.
- **chasquid/version** (string)  
  version string.
