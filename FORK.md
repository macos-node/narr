# This is a thin fork

Upstream is [`fiatjaf/narr`](https://github.com/fiatjaf/narr) and this fork
exists to stay close to it. It is not a project in its own right: nothing here
is a feature, there is no roadmap, and `master` is kept byte-identical to
`upstream/master` so that catching up is a fast-forward rather than a merge.

The reason it exists is that upstream has been quiet since **2025-04-27** while
its macOS releases are still Intel-only. On Apple silicon that means running
narr under Rosetta, which Apple has said stays as it is through macOS 26 and 27
and then narrows to a subset for old games. Waiting for a merge was not a plan.

## Branches

| Branch | What it is |
|---|---|
| `master` | upstream, untouched. Fast-forward it; never commit here. |
| `arm64-build` | the build fix alone — the branch to open a PR from. |
| `fork` | `arm64-build` plus the patches below. **Build and install from this one.** |

`upstream` is wired with its push URL set to `no-push`, so a stray `git push
upstream` fails instead of trying.

```sh
git fetch upstream
git checkout master && git merge --ff-only upstream/master
git rebase master arm64-build && git rebase arm64-build fork
```

## What diverges

**`arm64-build` — two lines, and the only part worth upstreaming.**

- `makefile`: `export GOARCH ?= amd64` → `$(shell go env GOARCH)`. That one
  line is the whole reason the releases are x86_64. `?=` already invited an
  override; it simply defaulted to the wrong thing. Cross-compiling still works
  by passing `GOARCH=` on the command line, exactly as before.
- `.github/workflows/build.yml`: the macOS job `macos-13` → `macos-14`.
  `macos-13` is GitHub's last Intel runner, so the runner had to move too or CI
  would keep shipping Intel whatever the makefile says.

narr needs **CGO** — `mattn/go-sqlite3` is C, `src/systray/systray_darwin.m` is
Objective-C against Cocoa — so a macOS build is native, not a cross-compile,
and the runner architecture has to match what it ships. Building arm64 *from*
an Intel Mac would want a cross C toolchain; from an Apple-silicon Mac there is
nothing to arrange.

A universal (`lipo`-joined) target would be the better thing for a *release*
and is deliberately not here, to keep the PR to two uncontroversial lines.

**`fork` — two patches from the findings in upstream issue
[#5](https://github.com/fiatjaf/narr/issues/5).**

Those findings are real; each was checked against the source before anything
was written here. They are *not* taken from upstream PR
[#6](https://github.com/fiatjaf/narr/pull/6), which is unreviewed, has its own
test plan unticked, and takes a different approach to the third finding.

- **`src/worker/limits.go` (new) and `src/worker/crawler.go`** — a ceiling on
  remote response bodies, 10 MB for feeds and pages, 1 MB for favicons. There
  was none, and the HTTP client caps time (30s overall) but not size.
  Two things to know about this patch:
  - It **fails** the read past the cap instead of truncating at it. Truncating
    is the tempting one-liner and it is worse than the bug, because a feed cut
    off mid-document parses as a malformed feed — the user is told their feed
    is broken when what happened is that it was too big.
  - It covers **five** sites, not the four that read with `io.ReadAll`. The
    fifth is `crawler.go`'s refresh path, which streams `res.Body` straight
    into `parser.ParseAndFix`. A sweep of the `ReadAll` calls misses it, and it
    is the most exposed of the five: it runs on a timer for every subscribed
    feed, with no user action at all.
  - `DiscoverFeed`'s second read is left alone on purpose. It reads from a
    `bytes.Reader` over the body already capped above it, so it is bounded.
- **`src/server/auth/auth.go`** — `HttpOnly` and `SameSite=Lax` on the session
  cookie, which had neither, so script in a feed article could read the session
  token. `Secure` is deliberately **not** set: narr is commonly served over
  plain HTTP on loopback or inside a LAN, where `Secure` stops the cookie being
  sent at all. TLS termination is the reverse proxy's job and it can add
  `Secure` itself.

  Note this one is inert unless narr is run with `-auth`, `-auth-file` or
  `NARR_AUTH`: without credentials the auth middleware is never installed
  (`src/server/routes.go`) and no cookie is ever set.

## What was deliberately not patched

**The SSRF on `/page`.** It is real — the handler takes `?url=` and fetches it
server-side with no validation, and the extracted content comes back in the
response. It is left alone because the available fix is worse than the exposure
here:

- the server binds `127.0.0.1` by default, so nothing off-machine reaches it;
- there are **no CORS headers anywhere** in narr, so a web page you visit can
  trigger a fetch but cannot read the result — the same-origin policy closes
  the drive-by path. *This is load-bearing: adding
  `Access-Control-Allow-Origin` for convenience would turn this into a real
  vulnerability.*
- blocking private and loopback ranges by resolved IP breaks legitimate
  self-hosted setups — a feed on your own LAN, or on the same box — with no
  useful error, which is a poor trade for a single-user loopback instance.

The honest residual risk: a feed item whose link points at an internal address
will be fetched if you open it in reader mode, and any local process can read
internal services through the endpoint. Running with `-auth` closes the second.

## Building on macOS

```sh
git checkout fork
make build_macos          # → _output/macos/narr.app, native to this machine
```

The bundle is ad-hoc signed by Go's linker as a side effect of
`GOARCH=arm64`. That matters: **Apple silicon refuses unsigned arm64 binaries
outright**, where unsigned x86_64 runs under Rosetta — and upstream ships narr
wholly unsigned, which is why its releases need a right-click to open and this
build does not.

`go test ./...` fails on `master` already — seven sanitizer expectations and
two test packages that no longer compile against their own source. Compare
against `master` before blaming a patch. Neither `src/worker` nor
`src/server/auth` has any test file upstream, so nothing here is covered.
