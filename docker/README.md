
# Docker

chasquid comes with a Dockerfile to create a container running [chasquid],
[dovecot], and managed certificates with [Let's Encrypt].

Note these are less thoroughly tested than the [traditional setup](howto.md),
which is the recommended way to use chasquid.

[chasquid]: https://blitiri.com.ar/p/chasquid
[dovecot]: https://dovecot.org
[Let's Encrypt]: https://letsencrypt.org


## Images

There are pre-built images at the
[gitlab registry](https://gitlab.com/albertito/chasquid/container_registry)
and [dockerhub](https://hub.docker.com/r/albertito/chasquid).
They are automatically built, and tagged with the corresponding branch name.
Use the *main* tag for a stable version.

If, instead, you want to build the image yourself, just run:

```sh
$ docker build -t chasquid -f docker/Dockerfile .
```

Or, if you are cross-compiling for a different architecture, e.g. `arm64`:

```sh
$ docker build --platform=linux/arm64 -t chasquid -f docker/Dockerfile .
```

## Running

First, pull the image into your target machine:

```sh
$ docker pull registry.gitlab.com/albertito/chasquid:main
```

You will need a data volume to store persistent data, outside the image. This
will contain the mailboxes, user databases, etc.

```sh
$ docker volume create chasquid-data
```

To add your first user to the image:

```
$ docker run \
	--mount source=chasquid-data,target=/data \
	-it --entrypoint=/add-user.sh \
	registry.gitlab.com/albertito/chasquid:main
Email (full user@domain format): pepe@example.com
Password:
pepe@example.com added to /data/dovecot/users
```

Upon startup, the image will obtain a TLS certificate for you using [Let's
Encrypt](https://letsencrypt.com/). You need to tell it the domain(s) to get a
certificate from by setting the `AUTO_CERTS` variable.

Because certificates expire, you should restart the container every week or
so. Certificates will be renewed automatically upon startup if needed.

In order for chasquid to get access to the source IP address, you will need to
use host networking, or create a custom docker network that does IP forwarding
and not proxying.

Finally, start the container:

```sh
$ docker run -e AUTO_CERTS=mail.yourdomain.com \
	--mount source=chasquid-data,target=/data \
	--network host \
	registry.gitlab.com/albertito/chasquid:main
```


## Debugging

To get a shell on the running container for debugging, you can use `docker ps`
to find the container ID, and then `docker exec -it CONTAINERID /bin/bash` to
open a shell on the running container.

