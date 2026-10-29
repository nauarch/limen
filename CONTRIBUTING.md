# Contributing to limen

## Build and test

```sh
go test ./...                                  # everything, architecture rules included
go vet ./...
gofmt -l .                                     # must print nothing
go generate ./internal/frontdoor/setup/dist    # limenctl binaries for limen to hand out
go build ./cmd/limen ./cmd/limenctl
```

CI runs the tests on Linux and Windows: limenctl runs on both, so a change to it should pass on
both. Tests that need a POSIX shell or PowerShell skip where there is none.

## Layout

limen is built as ports and adapters. The domain knows nothing of HTTP, Docker or files; the
outside world sits behind interfaces, and each binary is wired together in one place.

```
cmd/limen, cmd/limenctl     main packages: read flags, call the composition root
internal/
  contracts/                the ports: interfaces and plain types only, no logic
  kernel/                   small pure helpers every layer may use (errors, digests, sizes)
  features/<feature>/
    domain/                 the decisions, pure: no IO, no framework types
    app/                    use cases: run the domain against the ports
    compose.go              the feature's root: builds it from its options
  adapters/                 implementations of the ports (docker CLI, files, registries, disk)
  frontdoor/                how the outside reaches in (HTTP doors, the command line)
  server/, ctl/             composition roots of limen and limenctl
tests/                      mirrors internal/: tests/internal/x/y_test.go tests internal/x/y.go
  architecture/             the rules below, as tests
  kit/                      shared fakes of the ports
```

Before adding something, decide which layer it belongs to. Options are passed in, never read from
globals.

## Rules the tests enforce

`tests/architecture` fails the build when any of these is broken:

- **Standard library only.** `go.mod` requires no module. A dependency needs a strong case,
  discussed in an issue first.
- **Contracts are types.** `internal/contracts` holds interfaces, plain types and constants.
- **Pure layers stay pure.** `kernel` imports only the standard library; `contracts` adds the
  kernel; a feature's `domain` adds contracts and its own domain.
- **Features meet at their roots.** A feature imports another only through its root package.
  Adapters and front doors import no feature.
- **Tests mirror sources.** Every test file mirrors a source file, every package with logic has
  a test, and only `tests/kit` holds non-test code under `tests/`.
- **Every Go file carries a licence header**: a copyright line naming its author, then
  `// SPDX-License-Identifier: Apache-2.0`. The repository keeps its LICENSE and NOTICE.
- **Every Go file says its job.** Its first comment states what it is for. Comments describe the
  code as it is, never its history ("used to", "now", "previously").

Each test file opens with a `// Verifies:` list of the behaviours it checks. Fakes live in
`tests/kit` and implement the ports; prefer them to mocks.

## Commits

One change per commit, with a short message in this shape:

```
<type>: <what is true after the change, in plain words>
```

Types: `feat`, `fix`, `test`, `docs`, `style`, `refactor`, `build`, `ci`. The subject is lower
case, says what the code does now rather than what was done to it, and usually needs no body.
For example:

```
fix: a migration makes the folder for the saved images before saving them
feat: limenctl verify checks that a pull reaches limen
```

## Reporting

For a bug, say what you ran, what you expected and what happened, with the output of
`limenctl plan` or limen's log where it applies. Leave out anything private (addresses, image
names, tokens) that the report does not need.

## Licence

limen is licensed under Apache-2.0, and contributions are accepted under the same licence
(section 5). You keep the copyright in what you write:

- A file you create opens with your own name:

  ```go
  // Copyright 2026 Your Name
  // SPDX-License-Identifier: Apache-2.0
  ```

- In a file someone else wrote, keep its header as it is. Where you add a substantial part of
  it, you may add your own copyright line under the first.
- Never remove or change another author's copyright line, or the NOTICE.
