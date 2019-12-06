
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
avoided whenever possible, and will be noted in the [release
notes](relnotes.md).*

List of exported variables:

- **chasquid/aliases/hookResults** (hook result -> counter): count of aliases
  hook results, by hook and result.
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
- **chasquid/smtpIn/responseCodeCount** (code -> counter): count of response
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


## Prometheus

To monitor chasquid using [Prometheus](https://prometheus.io), you can use the
[prometheus-expvar-exporter](https://blitiri.com.ar/git/r/prometheus-expvar-exporter/b/master/t/f=README.md.html)
with the following configuration:

```toml
# Address to listen on. Prometheus should be told to scrape this.
listen_addr = ":8000"

[chasquid]
# Replace with the address of chasquid's monitoring server.
url = "http://localhost:1099/debug/vars"

# Metrics are auto-imported, but some can't be; in particular the ones with
# labels need explicit definitions here.

m.aliases_hook_results.expvar ="chasquid/aliases/hookResults"
m.aliases_hook_results.help ="aliases hook results"
m.aliases_hook_results.label_name ="result"

m.deliver_attempts.expvar = "chasquid/queue/deliverAttempts"
m.deliver_attempts.help = "attempts to deliver mail"
m.deliver_attempts.label_name = "recipient_type"

m.dsn_queued.expvar = "chasquid/queue/dsnQueued"
m.dsn_queued.help = "DSN queued"

m.items_written.expvar = "chasquid/queue/itemsWritten"
m.items_written.help = "items written"

m.queue_puts.expvar = "chasquid/queue/putCount"
m.queue_puts.help = "chasquid/queue/putCount"

m.smtpin_commands.expvar = "chasquid/smtpIn/commandCount"
m.smtpin_commands.help = "incoming SMTP command count"
m.smtpin_commands.label_name = "command"

m.smtp_hook_results.expvar = "chasquid/smtpIn/hookResults"
m.smtp_hook_results.help = "hook invocation results"
m.smtp_hook_results.label_name = "result"

m.loops_detected.expvar = "chasquid/smtpIn/loopsDetected"
m.loops_detected.help = "loops detected"

m.smtp_response_codes.expvar = "chasquid/smtpIn/responseCodeCount"
m.smtp_response_codes.help = "response codes returned to SMTP commands"
m.smtp_response_codes.label_name = "code"

m.in_sec_level_checks.expvar = "chasquid/smtpIn/securityLevelChecks"
m.in_sec_level_checks.help = "incoming security level check results"
m.in_sec_level_checks.label_name = "result"

m.spf_results.expvar = "chasquid/smtpIn/spfResultCount"
m.spf_results.help = "SPF result count"
m.spf_results.label_name = "result"

m.in_tls_usage.expvar = "chasquid/smtpIn/tlsCount"
m.in_tls_usage.help = "count of TLS usage in incoming connections"
m.in_tls_usage.label_name = "status"

m.out_sec_level_checks.expvar = "chasquid/smtpOut/securityLevelChecks"
m.out_sec_level_checks.help = "outgoing security level check results"
m.out_sec_level_checks.label_name = "result"

m.sts_modes.expvar = "chasquid/smtpOut/sts/mode"
m.sts_modes.help = "STS checks on outgoing connections, by mode"
m.sts_modes.label_name = "mode"

m.sts_security.expvar = "chasquid/smtpOut/sts/security"
m.sts_security.help = "STS security checks on outgoing connections, by result"
m.sts_security.label_name = "result"

m.out_tls_usage.expvar = "chasquid/smtpOut/tlsCount"
m.out_tls_usage.help = "count of TLS usage in outgoing connections"
m.out_tls_usage.label_name = "status"

m.sts_cache_expired.expvar = "chasquid/sts/cache/expired"
m.sts_cache_expired.help = "expired entries in the STS cache"

m.sts_cache_failed_fetch.expvar = "chasquid/sts/cache/failedFetch"
m.sts_cache_failed_fetch.help = "failed fetches in the STS cache"

m.sts_cache_fetches.expvar = "chasquid/sts/cache/fetches"
m.sts_cache_fetches.help = "total fetches in the STS cache"

m.sts_cache_hits.expvar = "chasquid/sts/cache/hits"
m.sts_cache_hits.help = "hits in the STS cache"

m.sts_cache_invalid.expvar = "chasquid/sts/cache/invalid"
m.sts_cache_invalid.help = "invalid policies in the STS cache"

m.sts_cache_io_errors.expvar = "chasquid/sts/cache/ioErrors"
m.sts_cache_io_errors.help = "I/O errors when maintaining STS cache"

m.sts_cache_marshal_errors.expvar = "chasquid/sts/cache/marshalErrors"
m.sts_cache_marshal_errors.help = "marshalling errors when maintaining STS cache"

m.sts_cache_refresh_cycles.expvar = "chasquid/sts/cache/refreshCycles"
m.sts_cache_refresh_cycles.help = "STS cache refresh cycles"

m.sts_cache_refresh_errors.expvar = "chasquid/sts/cache/refreshErrors"
m.sts_cache_refresh_errors.help = "STS cache refresh errors"

m.sts_cache_refreshes.expvar = "chasquid/sts/cache/refreshes"
m.sts_cache_refreshes.help = "count of STS cache refreshes"

m.sts_cache_unmarshal_errors.expvar = "chasquid/sts/cache/unmarshalErrors"
m.sts_cache_unmarshal_errors.help = "unmarshalling errors in STS cache"
```
