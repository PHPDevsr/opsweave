# Phase 04 shell execution proposal

Status `PROPOSED`. This is the next action contract after the accepted HTTP unit.
Runtime implementation requires review of the process/security boundary.

## Operator profiles and input

Implement `shell.exec` version `1` through the existing runtime. Default disabled.
Require `actions.shell.enabled` and a nonempty list of named command profiles.
Each profile defines an absolute executable path, absolute working directory and
fixed argument prefix. Limit profiles to 128. Reject duplicate names and invalid paths. No PATH search,
shell string expansion, environment interpolation or workflow-selected executable.
The YAML fields are `actions.shell.commands` (a list), with each entry containing
`name`, `executable`, `working_directory` and optional `args` (fixed prefix).
Only trusted administrators may control profile configuration, executable files
and working directories. Profiles grant the service OS account's existing rights.

Workflow inputs are required `command` (profile name) and optional `args` (array
of strings). Execute directly using `exec.CommandContext`. Maximum 64 combined
arguments and 8 KiB aggregate argument bytes; reject NUL and invalid UTF-8.
Arguments remain individual values, including shell metacharacters. Selecting an
interpreter or a tool capable of executing arbitrary code is an explicit operator
trust decision. Profiles grant the configured tool's full allowed argument surface;
they do not sandbox the tool. Operators must trust workflow authors and binaries.

Use LANG=C and LC_ALL=C on Unix with TMPDIR set to the OS temporary directory.
On Windows pass only SystemRoot/WINDIR and TEMP/TMP for OS runtime/temporary
files. Do not inherit PATH, HOME, USERPROFILE, application environment variables
or webhook credentials. No workflow
input for environment, stdin or credential values in this delivery. Never log argument
values, executable paths, environment or captured output in lifecycle logs.

## Output, limits and cancellation

Success returns integer `exit_code` (zero), UTF-8 `stdout` and `stderr`, each capped
at 8 KiB with an additional existing 64 KiB runtime JSON output limit. Nonzero exit,
invalid UTF-8, overflow and process-start failures fail the action with generic
errors and without persisting captured output. On overflow terminate the process
and report failure, without silent truncation. Never retain unbounded buffers.
Outputs and input arguments are ordinary persisted workflow data and must contain
no secrets. No internal retries; operator workflow retries can repeat side effects.

Use the earlier engine deadline or a 30-second action ceiling. On Unix, place the
command and cooperating children in an owned process group; on Windows, establish
an owned non-breakaway job before the configured executable starts. Terminate the
owned group/job on cancellation, deadline or overflow; close inherited capture
handles and reap the direct child. Bound post-cancellation waits to two seconds
and report cleanup failure. Unix profiles must not daemonize or move descendants
out of their group. This is a trusted-command contract, with no OS sandbox or
promise to contain arbitrary hostile binaries. Do not claim exactly-once effects.
Native implementation must support Linux, macOS and Windows and preserve
dispatcher shutdown ownership.

## Verification and documentation

Add isolated executable-helper tests for valid argv, metacharacter preservation,
missing/disabled profiles, invalid input, nonzero exit, output/JSON limits,
cancellation/deadline, child-tree cleanup and no inherited credential environment.
Exercise production CLI-to-history and queued/server paths. Run full required Go,
race and Markdown checks, review security, measure an isolated benchmark in CI,
and document profile configuration, limits and platform caveats. No storage
migration or new dependency is planned; native process controls may use the
existing x/sys dependency.

## Decision requested

Accept named operator-controlled executable profiles, direct argument arrays,
minimal environment, bounded persisted output and owned process-tree termination.
This authorizes only `shell.exec`; Docker, filesystem and notification contracts
and Phase 05 require separate review.
