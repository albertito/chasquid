
# Testing

## Go tests

All Go packages have their own test suite, which provides easy and portable
tests with decent enough coverage.


## Integration tests

In the `test/` directory there is a set of end to end integration tests,
written usually in a combination of bash and Python 3.

They're not expected to be portable, as that gets impractical very quickly,
but should be usable in most Linux environments.

They provide critical coverage and integration tests for real life scenarios,
as well as interactions with other software (like Exim or Dovecot).


### Dependencies

The tests depend on the following things being installed on the system (listed
as Debian package, for consistency):

 - `msmtp`
 - `util-linux` (for `/usr/bin/setsid`)

Some individual tests have additional dependencies, and the tests are skipped
if the dependencies are not found:

- `t-02-exim` Exim interaction tests:
    - `gettext-base` (for `/usr/bin/envsubst`)
    - The `exim` binary available somewhere, but it doesn't have to be
      installed.  There's a script `get-exim4-debian.sh` to get it from the
      archives.
- `t-11-dovecot` Dovecot interaction tests:
    - `dovecot`
- `t-15-driusan_dkim` DKIM integration tests:
    - The `dkimsign dkimverify dkimkeygen` binaries, from
      [driusan/dkim](https://github.com/driusan/dkim) (no Debian package yet).
- `t-18-haproxy` HAProxy integration tests:
    - `haproxy`

For some tests, python >= 3.5 is required; they will be skipped if it's not
available.


## Stress tests

Also in the `test/` directory there is a set of stress tests, which generate
load against chasquid to measure performance and resource consumption.

While they are not exhaustive, they are useful to catch regressions and track
improvements on the main code paths.

## Fuzz tests

Some Go packages also have instrumentation to run fuzz testing against them,
with the [go-fuzz](https://github.com/dvyukov/go-fuzz) tool.

This is critical for packages that handle sensitive user input, such as
authentication encoding, aliases files, or username normalization.

They are implemented by a `fuzz.go` file within their respective Go packages.


## Command-line tool tests

Each command-line tool has their own set of tests, see the `test.sh` file on
their corresponding directories.


## Docker

The `test/Dockerfile` can be used to set up a suitable isolated environment to
run the integration and stress tests.

This is very useful for automated tests, or running the integration tests in
constrained or non supported environments.


## Automated tests

There are two sets of automated tests which are run on every commit to
upstream, and weekly:

* [GitLab CI](https://gitlab.com/albertito/chasquid/commits/master),
  configured in the `.gitlab-ci.yml` file, runs the Go tests and the
  integration tests (using [docker](#docker)).  
  The integration tests are run twice: once against the dependencies listed in
  `go.mod`, and once against the latest version of the dependencies.
  It also builds the [public Docker images](docker.md).

* [Cirrus CI](https://gitlab.com/albertito/chasquid/pipelines),
  configured in the `.cirrus.yml` file, runs Go tests on FreeBSD, and a
  comprehensive linter.


## Coverage

The `test/cover.sh` script runs the integration tests in coverage mode, and
produces a code coverage report in HTML format, for ease of analysis.

Unfortunately, exiting with any of the *Fatal* functions does not save
coverage output. Those paths are very important to test, but don't expect to
see them reflected in the coverage report for now.

The target is to keep coverage of the `chasquid` binary above 90%.
