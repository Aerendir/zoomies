---
icon: material/scale-balance
title: "Zoomies vs GARM (GitHub Actions Runner Manager)"
description: >-
  How Zoomies compares with GARM: two single-binary, SQLite-backed runner
  managers with a web UI, and where each is the better choice.
---

# Zoomies and GARM

[GARM](https://github.com/cloudbase/garm) (GitHub Actions Runner Manager) is the
closest project to Zoomies in shape, and an honest comparison starts there: both
are a single binary that keeps its state in SQLite, both have a built-in web UI,
and neither needs Kubernetes. If you are choosing between them, the shared
design is not the difference.

This page was written from GARM's public documentation, reviewed in October
2026. GARM's README warns that its main branch can describe work that is not in
a stable release, so check a release's own documentation before relying on a
feature here.

## Side by side

| | GARM | Zoomies |
| --- | --- | --- |
| Model and licence | Self-hosted; Apache-2.0 | Self-hosted; [AGPL-3.0](https://github.com/eyupio/zoomies/blob/main/LICENSE) |
| Shape | One binary, SQLite, a web UI | One binary, SQLite, a [web UI](../ui.md) |
| Where runners come from | External providers, which create and delete the machines | Containers on hosts you add, or machines rented through a [provider](../providers.md), with [Proxmox VE](../proxmox.md) first |
| How it learns of jobs | Webhook-driven pools, and native scale sets | `workflow_job` webhooks, with polling as a fallback |
| Forges | GitHub, GitHub Enterprise Server and Gitea | GitHub. Enterprise Server is configurable but [not yet tested](../faq.md#does-it-work-with-github-enterprise-server) |
| Providers | Standalone executables for AWS, Azure, GCP, OpenStack, OCI, LXD, Incus, Kubernetes and others; you can write one in any language | Containers on hosts you add, and a [provider contract](../providers.md) with Proxmox VE first |
| Scaling | Webhook pools with round-robin or pack balancing, GitHub's runner scale sets, and pools that scale to zero | Pools reconciled against queued jobs, scaling to zero by default |
| Moving workflows onto it | Not verified | A [migration wizard](../migration.md) that opens one pull request per repository |
| Machines at home or behind NAT | Not verified | Agents dial out, so nothing is opened; [Tailcat](../private-hosts.md) brings in private hosts |

## When GARM is the better choice

- **You use Gitea**, or run GitHub Enterprise Server in production and want a
  tool that already documents it.
- **You want the provider model.** GARM creates and deletes machines through
  external providers, so a cloud or hypervisor with a provider is an option
  today. Zoomies' provider contract is newer and has fewer providers.
- **You want native scale sets.** GARM describes them; Zoomies uses webhooks and
  a fallback poller, and is still assessing scale sets.

## When Zoomies is the better choice

- **You are moving a lot of repositories.** The
  [migration wizard](../migration.md) shows the diff and opens one pull request
  per repository, and it can give a pool a label you choose so static runners are
  replaced [without editing workflows](../migration.md#coming-from-your-own-static-runners).
- **Your capacity is private hardware.** An agent on a home-lab box or an office
  machine joins by dialling out, with no inbound rule. See
  [runners in your home lab](../home-lab.md).
- **You want to run it from a browser, with reasons.** The UI says in plain
  words why the scheduler did what it did and keeps an audit log of every change.

Neither claim is a statement that Zoomies runs faster or isolates better; this
project has not measured either against GARM.

## What GARM's provider model gives you

A provider is a separate program that creates and deletes the machine, so GARM reaches whatever a provider can: a
public cloud, a private OpenStack, an LXD or Incus host. If your capacity is
spread across clouds that Zoomies has no provider for, that breadth is the reason
to choose it. Zoomies has the Docker and Podman hosts it manages directly, and
[Proxmox VE](../proxmox.md) as its first rented-machine provider.

## Moving between them

Pools and runners are named by label, so the two can run side by side when their
labels differ. A sensible order:

1. Add a Zoomies pool with its own label.
2. Move one repository's `runs-on` to it. The [migration wizard](../migration.md)
   opens the pull request.
3. Compare queue time and duration on that repository against the same jobs on
   GARM.
4. Move the rest one repository at a time, then retire the GARM pool once nothing
   targets its label.

Neither tool changes your workflow beyond the `runs-on` line, so moving back is
the same steps in reverse.

## Where to go next

- [Quick start](../quickstart.md): a fresh host to a running job in about five
  minutes.
- [Architecture](../architecture.md): why Zoomies is built the way it is.
- [All alternatives](index.md)
