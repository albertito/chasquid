# NAME

chasquid-util - chasquid management tool

# SYNOPSIS

**chasquid-util** \[_options_\] user-add _user@domain_ \[--password=_password_\]

**chasquid-util** \[_options_\] user-remove _user@domain_

**chasquid-util** \[_options_\] authenticate _user@domain_ \[--password=_password_\]

**chasquid-util** \[_options_\] check-userdb _domain_

**chasquid-util** \[_options_\] aliases-resolve _addr_

**chasquid-util** \[_options_\] domaininfo-remove _domain_

**chasquid-util** \[_options_\] print-config

# DESCRIPTION

chasquid-util is a command-line utility for [chasquid(1)](chasquid.1.md) operations.

# OPTIONS

- **user-add** _user@domain_ \[--password=_password_\]

    Add a new user to the domain.

- **user-remove** _user@domain_

    Remove the user from the domain.

- **authenticate** _user@domain_ \[--password=_password_\]

    Check the user's password.

- **check-userdb** _domain_

    Check the integrity of the domain's users database.

- **aliases-resolve** _addr_

    Resolve the given address. Talks to the running chasquid instance.

- **domaininfo-remove** _domain_

    Remove the domain information entry. This can be used to manually allow a
    security level downgrade. Talks to the running chasquid instance.

- **print-config**

    Parse and print the configuration in a human-readable way.

- **-C** or **--configdir=&lt;path**>

    Configuration directory.

# SEE ALSO

[chasquid(1)](chasquid.1.md)
