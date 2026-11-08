# Install a release binary

Download a published version from [OpsWeave Releases](https://github.com/PHPDevsr/opsweave/releases).
You do not need Go, Bun, Docker, Redis or RabbitMQ to run the binary. This is
development software: the production action catalog is still empty, so binaries
currently provide administration/history commands rather than usable built-in
workflow actions. Packaging does not imply later roadmap features are complete.

## Choose a download

Replace `<version>` with the selected release tag, including `v`.

| System  | CPU              | Archive                                  |
| ------- | ---------------- | ---------------------------------------- |
| Linux   | Intel/AMD 64-bit | `opsweave_<version>_linux_amd64.tar.gz`  |
| Linux   | ARM 64-bit       | `opsweave_<version>_linux_arm64.tar.gz`  |
| Windows | Intel/AMD 64-bit | `opsweave_<version>_windows_amd64.zip`   |
| Windows | ARM 64-bit       | `opsweave_<version>_windows_arm64.zip`   |
| macOS   | Intel            | `opsweave_<version>_darwin_amd64.tar.gz` |
| macOS   | Apple Silicon    | `opsweave_<version>_darwin_arm64.tar.gz` |

Each archive contains `opsweave` (or `opsweave.exe`) and this `INSTALL.md`.
Download `checksums.txt` from the same release and compare the SHA-256 hash before
extracting. Checksums detect corruption; they are not an independent publisher
signature. Current binaries are not Authenticode-signed or Apple-notarized.
Platform security prompts may require your normal trusted-software approval.

## Linux and macOS

For example, after downloading the archive for your platform:

```sh
# Linux; select the line matching the archive you downloaded.
sha256sum opsweave_<version>_linux_amd64.tar.gz
# macOS equivalent:
shasum -a 256 opsweave_<version>_darwin_arm64.tar.gz
```

Compare that hash with its exact filename in `checksums.txt`. Then extract your
archive using `tar -xzf <archive>` and run `./opsweave version` and
`./opsweave --help`. Optionally move the binary to a directory on your PATH.
Unix archives preserve executable mode. Linux binaries are built with CGO disabled;
SQLite is compiled into the Go binary, with no separate database server.

## Windows

In PowerShell, compare the displayed hash with `checksums.txt`:

```powershell
Get-FileHash .\opsweave_<version>_windows_amd64.zip -Algorithm SHA256
Expand-Archive .\opsweave_<version>_windows_amd64.zip -DestinationPath .\opsweave
Set-Location .\opsweave
.\opsweave.exe version
.\opsweave.exe --help
```

Use the arm64 archive instead for Windows on ARM. Add the containing directory
to PATH if desired. The executable does not require a C compiler or a separate
SQLite DLL. Do not substitute an archive for another operating system or CPU.

## Configuration and upgrades

Run `opsweave init` in your chosen working directory to create configuration
without overwriting an existing file. `opsweave workflow list` opens the configured
SQLite database and applies migrations. The default path is `./data/opsweave.db`;
`OPSWEAVE_STORAGE_PATH` selects another local path. Keep this data outside the
extracted release directory when replacing binaries. Help/version/completion do
not require configuration or storage.

Stop OpsWeave and back up its database and configuration before upgrading. Read
the selected release notes for migration and compatibility changes. Restore the
pre-upgrade database backup with the compatible older binary to roll back; do not
run an older binary against a newer schema. Replacing a binary does not roll back
a database migration. Existing unfinished executions are not automatically replayed.

The repository's `docs/foundation.md` and `docs/execution.md` provide full current
configuration and execution details. For released behavior, inspect documentation
at the exact release tag rather than newer `main`. No separately versioned source
directories or documentation deployment are introduced by binary packaging.
