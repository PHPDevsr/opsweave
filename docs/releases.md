# Release workflow and maintainer procedure

The [Release workflow](../.github/workflows/release.yml) builds Linux,
Windows and macOS packages for amd64 and arm64. The standard-library
`tools/release` command creates versioned tar.gz/zip files and SHA-256 sidecars.
It uses Go from `go.mod`, CGO disabled, trimpath, no embedded local VCS metadata,
and a linker-assigned version. The ordinary source build still reports `dev`.
Archive entry ordering, timestamps and modes are deterministic; this does not
claim notarization, publisher signatures or cross-toolchain bitwise equivalence.

## Before the first release

This workflow is an explicitly requested packaging extension before Phase 03,
not approval to start Phase 03 or qualify the whole product for production.
Choose a release version only after reviewing its actual delivered capabilities,
tests and open quality gates. The repository CodeQL upload configuration issue
remains open until an owner enables the necessary code-scanning feature and a
run passes; release packaging does not resolve or disable that gate.

There are currently no release tags. Adding CI does not publish a release or
promise that a first release already exists. No signing/notarization credentials,
package-manager formula, container image or documentation website is added.

## Test without publishing

Pull requests changing Go, modules, installation instructions or the workflow run
all six packaging jobs. `workflow_dispatch` also makes a preview build of the selected
ref. Both use version `v0.0.0-preview`, store 14-day Actions artifacts, and cannot
publish a GitHub Release. To try packaging locally from the repository root:

```sh
go test ./tools/release
go run ./tools/release -version v0.0.0-preview -out dist
```

The helper defaults to the native OS/CPU; explicit `-goos` and `-goarch` flags
select a supported cross-build target. Output files use exclusive creation: rerunning into the same
directory fails rather than overwriting an archive. Use a new output directory.
No global packaging dependency is installed. `dist/` is ignored by Git.

Each matrix job verifies its build host, modules, vet and full native test suite,
then packages and extracts the target binary. All six verify checksums and target
build metadata. Linux/Windows amd64 and both macOS targets also execute the binary
to check embedded version, help and SQLite creation/migrations via `execution list`.
Because this repository is private, Linux/Windows arm64 are cross-built on amd64
runners; their binaries need runtime qualification on ARM hardware before claiming
native ARM support. No paid ARM runner or billing change is configured.
The assemble job requires all six archives and checksums, verifies every hash,
and emits one `release-bundle` containing six archives and `checksums.txt`.
Read-only permissions apply to builds, PRs and previews.

## Publish from an intentional tag

After merging the reviewed workflow and code to `main`, resolving applicable
release gates and choosing the version, a maintainer may create an annotated tag
on the intended commit and push it. For example, replace the version/commit below:

```sh
git tag -a v0.1.0-rc.1 <reviewed-commit> -m "OpsWeave v0.1.0-rc.1"
git push origin v0.1.0-rc.1
```

Supported tags are `vMAJOR.MINOR.PATCH` with optional SemVer prerelease identifiers
such as `-rc.1`; numeric identifiers cannot have leading zeros. Build metadata is
not accepted. A tag must point to a commit reachable from `main`. The workflow
checks out that exact ref and publishes only after every packaging build and complete
archive verification passes. Tag/ref values enter scripts through environment
variables and are validated before interpolation into filenames/linker arguments.

Only the tag-triggered publish job receives `contents: write`. It downloads the
bundle from the same workflow run and rechecks hashes before calling
`gh release create --verify-tag --generate-notes`. The command creates a draft,
uploads assets and then publishes; prerelease tags use `--prerelease --latest=false`.
Stable tags use GitHub's automatic latest-release selection. No tag is created by
the workflow. The remote tag must still resolve to the exact built commit immediately
before publication. Existing releases/assets are never uploaded with `--clobber` or
silently replaced. A rerun against an existing release fails and requires explicit
maintainer inspection, including any draft left after an upload failure.

Concurrency is serialized per ref without cancelling an in-progress publication.
Full SHA pins are used for checkout, Go setup and artifact upload/download.
Runner availability or repository Actions restrictions can block a target;
that failure blocks publication rather than silently reducing the release matrix.

The single documentation source remains in Git. Released documentation must come
from the exact tag, preserve previous versions and route stable/development
versions separately when the later documentation deployment feature is delivered.
This workflow publishes binaries and embedded installation instructions only.
