# Keeping a host at its recommended size

Date: 19 October 2026.

## Why

A CI job failed with `no space left on device` on a rented Proxmox machine whose
capacity and reserve were not the ones **Adjust** recommends. The dialog already
knows the right numbers; nothing applied them, and nothing counted the disk when
deciding how many runners a machine holds.

## Decisions

* **What is adjusted:** capacity (the number of runners) and the CPU, memory and
  disk reserves. Not the operating system: host tuning stays consent-based and
  never unattended (`internal/hosttune/AGENTS.md`).
* **The disk is a budget as well as a reserve.** Capacity is the smallest of the
  CPU, memory and disk counts, with 10 GiB of working room per runner
  (`scheduler.AutoAdjustRunnerDiskMB`). It is a constant for now; the audit row and
  the page say when it was the binding figure.
* **One source of truth.** The recommendation is a pure function in
  `internal/scheduler/recommend.go`, held to the same cases as
  `web/src/lib/hosts/recommend.ts`. `GET /hosts/{id}/recommendation` serves it, and the
  dialog uses it where the disk is the binding figure.
* **A per-host choice with a fleet default.** `host_auto_adjust` in its own table
  (`on`, `off`, or no row to follow `scheduler.host_auto_adjust`, off by default),
  so the central host row and its queries do not change. Provider-rented hosts are
  hosts, so the fleet default reaches them.
* **Guardrails.** Never un-pause (capacity 0), never touch a cordoned, retired,
  incompatible or silent host, five minutes between moves, and the same
  stranded-pool refusal a person gets. Every move is audited under the system's name.

## Not done

A runtime response to a disk that is filling now (a throttle rung for disk
pressure, or pruning) is a separate piece. The reserve already stops new runners
starting when free disk is at or below it, and this keeps the reserve right.
The 10 GiB allowance is not yet a setting.
