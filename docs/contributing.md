
# Contributing

## Questions and bug reports

Please send questions and bug reports via [Github Issues], or to the [mailing
list], chasquid@googlegroups.com.  
To subscribe to the [mailing list], send an email to
`chasquid+subscribe@googlegroups.com`.

To privately report a suspected security issue, please see [Reporting a
security issue](security.md).

<a name="irc"></a>
You can also reach out via IRC, `#chasquid` on [OFTC](https://oftc.net/).


## Patches and pull requests

You can send patches and pull requests via [Github Pull requests], or the
[mailing list].

Before sending any non-trivial change, it's ideal to discuss the proposal (via
[Github Issues], [mailing list], or [IRC](#irc)). This helps make the most out
of the contribution, and minimize friction frustration during code reviews.


### Commit message, coding style, tests

Ideally, patches would have descriptive commit messages, adhere to Go's coding
style, and include comprehensive tests.  However, that is a lot to ask, and
very subjective.

It is okay to propose patches without those things. In those cases, the
maintainer will usually amend the patches to add them.

For how to write commit messages,
[this](https://chris.beams.io/posts/git-commit/) and
[this](https://git-scm.com/docs/SubmittingPatches#describe-changes) articles
contain great advise.


### Workflow

Patches will be cherry-picked, and are typically first incorporated into the
`next` branch.

The `next` branch is where patches are staged, tested and reviewed. This
branch is rebased frequently, to make adjustments and fixes as needed.
Once patches have received enough testing (usually after a couple of weeks),
they are moved to the `main` branch.

The `main` branch is the stable branch and where releases are cut from. It is
never rebased.


[Github Issues]: https://github.com/albertito/chasquid/issues
[mailing list]: https://groups.google.com/forum/#!forum/chasquid
[GitHub's Security tab]: https://github.com/albertito/chasquid/security
[Github Pull requests]: https://github.com/albertito/chasquid/pulls
