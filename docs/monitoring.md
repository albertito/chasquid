
# Monitoring

chasquid includes an HTTP server for monitoring purposes, which for security
it is not enabled by default.

You can use the `monitoring_address` configuration option to enable it.

Then just browse the address and human-friendly links to various monitoring
and debugging tools should appear.

These include:

- Command-line flags.
- [Traces](https://godoc.org/golang.org/x/net/trace) of both short and long
  lived requests (sampled).
- State of the queue.
- State of goroutines.
- [Exported variables](#variables).
- Profiling endpoints, for use with `go tool pprof` or similar tools.


## Variables

chasquid exports some variables for monitoring, via the standard
[expvar](https://golang.org/pkg/expvar/) package, which can be useful for
whitebox monitoring.

They're accessible over the monitoring http server, at `/debug/vars` (default
endpoint for expvars).

*Note these are still subject to change, although breaking changes will be
avoided whenever possible, and will be noted in the [upgrading
notes](../UPGRADING.md).*

List of exported variables:

- **chasquid/queue/deliverAttempts** (recipient type -> counter): attempts to
  deliver mail, by recipient type (pipe/local email/remote email).
- **chasquid/queue/dsnQueued** (counter): count of DSNs that we generated
  (queued).
- **chasquid/queue/itemsWritten** (counter): count of items the queue wrote to
  disk.
- **chasquid/queue/putCount** (counter): number of envelopes put in the queue.
- **chasquid/smtpIn/commandCount** (map of command -> count): count of SMTP
  commands received, by command. Note that for unknown commands we use
  `unknown<COMMAND>`.
- **chasquid/smtpIn/hookResults** (result -> counter): count of hook
  invocations, by result.
- **chasquid/smtpIn/loopsDetected** (counter): count of email loops detected.
- **chasquid/smtpIn/responseCodeCount** (result -> counter): count of response
  codes returned to incoming SMTP connections, by result code.
- **chasquid/smtpIn/securityLevelChecks** (result -> counter): count of
  security level checks on incoming connections, by result.
- **chasquid/smtpIn/spfResultCount** (result -> counter): count of SPF checks,
  by result.
- **chasquid/smtpIn/tlsCount** (tls status -> counter): count of TLS statuses
  (plain/tls) for incoming SMTP connections.
- **chasquid/smtpOut/securityLevelChecks** (result -> counter): count of
  security level checks on outgoing connections, by result.
- **chasquid/smtpOut/sts/mode** (mode -> counter): count of STS checks on
  outgoing connections, by mode (enforce/testing).
- **chasquid/smtpOut/sts/security** (result -> counter): count of STS security
  checks on outgoing connections, by result (pass/fail).
- **chasquid/smtpOut/tlsCount** (status -> counter): count of TLS status
  (insecure TLS/secure TLS/plain) on outgoing connections.
- **chasquid/sourceDateStr** (string): timestamp when the binary was built, in
  human readable format.
- **chasquid/sourceDateTimestamp** (int): timestamp when the binary was built,
  in seconds since epoch.
- **chasquid/sts/cache/expired** (counter): count of expired entries in the
  STS cache.
- **chasquid/sts/cache/failedFetch** (counter): count of failed fetches in the
  STS cache.
- **chasquid/sts/cache/fetches** (counter): count of total fetches in the STS
  cache.
- **chasquid/sts/cache/hits** (counter): count of hits in the STS cache.
- **chasquid/sts/cache/invalid** (counter): count of invalid policies in the
  STS cache.
- **chasquid/sts/cache/ioErrors** (counter): count of I/O errors when
  reading/writing as part of keeping the STS cache.
- **chasquid/sts/cache/marshalErrors** (counter): count of marshaling errors
  as part of keeping the STS cache.
- **chasquid/sts/cache/refreshCycles** (counter): count of STS cache refresh
  cycles.
- **chasquid/sts/cache/refreshErrors** (counter): count of STS cache refresh
  errors.
- **chasquid/sts/cache/refreshes** (counter): count of STS cache refreshes.
- **chasquid/sts/cache/unmarshalErrors** (counter): count of unmarshaling
  errors as part of keeping the STS cache.
- **chasquid/version** (string): version string.

