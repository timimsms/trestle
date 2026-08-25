# Security

## Scope

Trestle is a local CLI. It reads files in a repository you already have checked out, and it makes
no network calls — no telemetry, no update check, no remote fetch of any kind. There is no server,
no account, and nothing to log into.

That shape limits what a vulnerability here can be. The realistic categories:

- **Path traversal**: a crafted `.trestle.yml` or binding glob causing reads outside the repo root.
- **Denial of service**: input that makes `check` hang or exhaust memory, which matters because it
  is designed to run in CI on every pull request.
- **Untrusted input handling**: Trestle parses `.d2` files and YAML. It compiles diagrams through
  [D2](https://github.com/terrastruct/d2), so a crash reachable only through that library is worth
  reporting to us *and* upstream.

Running `trestle` on a repository you do not trust is running a parser on untrusted input. That is
worth knowing before you point it at somebody else's clone.

## Reporting

Open a [private security advisory](https://github.com/timimsms/trestle/security/advisories/new).

If you would rather not use GitHub for it, open a normal issue saying only that you have something
to report, with no detail, and we will find a channel.

Please include a reproduction if you can — a minimal repo tree, the config and the command. This
project's whole habit is verifying claims rather than reasoning about them, and a reproduction is
what makes that possible.

Expect an acknowledgement within a week. This is a small project maintained in spare time; that is
the honest timeline rather than an aspirational one.

## Supported versions

The latest tagged release. There is no long-term support branch and no backporting — at this size,
saying otherwise would be a promise nobody can keep.
