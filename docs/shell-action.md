# Configured shell execution

`shell.exec` version `1` supports `opsweave.io/v1` through the existing executor,
engine and dispatcher. It is disabled by default and grants no executable access
until an administrator configures command profiles. The action invokes executables
directly with argument arrays using exec.CommandContext. No shell string, PATH
lookup, variable expansion or input-selected executable is introduced.

## Configure an operator profile

A Linux/macOS example using an existing local executable and working directory:

```yaml
actions:
  shell:
    enabled: true
    commands:
      - name: print-message
        executable: /usr/bin/printf
        working_directory: /srv/opsweave
        args: ['%s\n']
```

Choose paths appropriate to the host before running this example. On Windows,
use an absolute executable such as `C:\Windows\System32\hostname.exe` and a local
working directory, with an empty argument prefix. Profiles are configured in YAML;
`OPSWEAVE_SHELL_ENABLED=true` or `false` overrides only enablement. Enabling without
a nonempty valid profile list is rejected. Existing configurations remain valid
with shell disabled. HTTP and shell enablement and permissions are independent.

Each entry requires `name`, `executable` and `working_directory`. Optional `args`
is the fixed argument prefix, default empty. Profile names are case-sensitive,
1-64 ASCII letters/digits/dots/hyphens/underscores, excluding `.` and `..`.
Duplicate names and unknown fields are rejected. Limit profiles to 128.
Executable and directory paths must be absolute, local and valid UTF-8, with no
NUL, UNC/device path or extra colon beyond a Windows drive prefix. Executables
must be regular files; Unix executable bits must be present. Working directories
must exist. Validate configured paths even when disabled; remove obsolete profiles
when removing their binaries. Invocation rechecks paths before starting a process.

Administrators must control profile configuration, binaries and working directories.
Symlinks and binary contents are trusted operator assets; profiles do not pin an
executable hash. Changing a profile changes future invocations of that alias,
including existing workflow revisions, without rewriting stored workflow source.

## Workflow and CLI

```yaml
apiVersion: opsweave.io/v1
kind: Workflow
metadata:
  name: print-maintenance-message
spec:
  steps:
    - id: message
      uses: shell.exec
      with:
        command: print-message
        args: [maintenance-ready]
```

```sh
opsweave validate workflow.yaml
opsweave apply workflow.yaml
opsweave run print-maintenance-message
opsweave execution get <execution-id>
```

`command` is required and selects an exact configured profile name. `args` is an
optional array of strings appended to its fixed prefix. Reject unknown inputs,
non-string arguments, NUL and invalid UTF-8. The combined prefix and workflow
arguments have a 64-entry and 8192-byte total limit, excluding the executable path.
Arguments with spaces, quotes or shell metacharacters remain individual argv
values. Preflight validates descriptor types without executing a command; exact
profile selection and semantic argument bounds are checked at invocation.

Queued execution, retries, schedules and server shutdown use the same action
runtime. `run --enqueue` submits durable work; `opsweave server` drains it using
its current configuration. Disabling shell revokes new invocations while stored
execution history remains readable with a valid configuration. Exact action
version binding remains `1`; no database migration or new dependency is needed.

## Environment and trust boundary

- Unix commands receive only LANG=C, LC_ALL=C and TMPDIR from the OS temporary
  directory.
- Windows commands receive only SystemRoot/WINDIR from the Windows directory API,
  plus TEMP/TMP from the OS temporary directory.
- PATH, HOME, USERPROFILE, webhook secrets and arbitrary parent environment values
  are not inherited. There is no workflow environment, credential or stdin input.
  Programs depending on PATH/HOME should be replaced with suitable foreground
  tools or explicit absolute-path arguments through the operator profile.

The executable runs with the service OS account's existing rights and resource
access. A profile permits the tool's entire accepted argument surface. Choosing
an interpreter or a program that can execute code is an explicit operator trust
decision. Tool-specific option parsing still applies; use a fixed `--` separator
when the chosen tool supports it for untrusted data arguments. There is no OS
filesystem, CPU or memory sandbox. Trust workflow authors
and use a least-privileged service account. Programs must retain controllable
process ownership; avoid daemonization, credential/privilege changes and external
launch brokers that create processes outside the owned group/job.

All input arguments and successful outputs are ordinary persisted data. Never put
secrets in workflow arguments, fixed prefixes, stdout or stderr. Encrypted secret
resolution is separate work. Lifecycle logs include existing action/execution/step
identities and omit command paths, argument values, environment and captured data.

## Results, bounds and process cleanup

Success returns `exit_code: 0`, `stdout` and `stderr`. Capture each stream up to
8192 bytes and validate UTF-8. The existing runtime additionally limits serialized
JSON output to 65536 bytes, including escaping. Oversized, invalid UTF-8 or excessive
escaped output fails without silently truncating a successful result. Output
buffers remain bounded even when the executable writes continuously.

Nonzero exit, unavailable profile/path, process-start/ownership failure, overflow
and capture/cleanup failure fail the action through the existing generic error
boundary. Failed output is not persisted. Cancellation/deadline propagation retains
engine behavior. A timeout or failed action cannot undo external side effects;
configured workflow retries can repeat them. No retries happen inside this action.

Use the earlier engine deadline or a 30-second action ceiling. On cancellation,
deadline or overflow, stop the owned process group/job and reap the direct process.
Native cleanup shares a two-second post-cancellation budget, with bounded stream
waits and no detached capture goroutines. An exited root that leaves descendants
holding its capture handles fails after bounded waiting, and cleanup still stops
the owned descendants.

On Linux/macOS, establish a new process group. Trusted commands and their children
must stay in that group. On Windows, create the command suspended, assign it to an
unnamed non-breakaway job with kill-on-close, validate the initial thread's owning
PID and resume only after assignment succeeds. Assignment failures kill/reap the
suspended root. No job handle is inherited by the configured executable. Windows
job accounting is checked before closing the ownership handle. Reference
[Microsoft job object behavior](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).

## Troubleshooting and compatibility

If the action is unavailable, verify enablement and the exact version binding in
both administrator and server configurations. If a profile is invalid, check its
absolute local paths, file type, executable permissions, directory and argument
bounds. Windows batch files and Unix scripts requiring a missing interpreter/PATH
can fail at process start; the action never inserts an interpreter automatically.

Generic failures deliberately omit native errors and captured output. Check the
configured program and service account outside secret-bearing workflow history.
For Windows job-assignment restrictions, use a compatible native host; the action
fails closed before running an unowned executable. Profile revocation does not
change stored revisions. Stop active executors before replacing binaries; earlier
OpsWeave binaries cannot invoke this newly added action version. No database
conversion or application dependency update accompanies this feature.
