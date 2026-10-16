---
icon: material/cloud-sync-outline
title: "Zoomies vs Cirun"
description: >-
  How Zoomies compares with Cirun: a managed control plane against free software
  you run yourself. What each costs, who holds the control plane, when each
  fits.
---

# Zoomies and Cirun

[Cirun](https://docs.cirun.io/) is a managed service. It provisions runners in
your own cloud account, or on your hardware through agents, and the control plane
is theirs. Zoomies is free software where the control plane is yours too. That
difference, rather than a feature list, is the honest place to start.

Written from Cirun's public documentation and pricing page, reviewed in
September 2026. Prices change; check [Cirun's site](https://cirun.io/) first.

## Side by side

| | Cirun | Zoomies |
| --- | --- | --- |
| Model | A managed service; open-source components do not make the hosted control plane open source | Self-hosted; [AGPL-3.0](https://github.com/eyupio/zoomies/blob/main/LICENSE) |
| Software fee | Free for open-source repositories; at review, $29 a month for 3 private repositories and $79 for 10, with infrastructure extra | None |
| Where runners run | Your cloud account, or on-premises machines through agents | Hosts you own or rent; [Proxmox VE](../proxmox.md) as a provider |
| Private hardware | Supported, with Linux and macOS VM and Docker executors documented | Supported; agents dial out and [Tailcat](../private-hosts.md) brings in private hosts |
| Windows | Documented on cloud runners | An agent that is [not yet qualified](../index.md#what-is-qualified) |
| Caching | Automatic on Linux with AWS, and a separate S3-compatible action | [Persistent caches](../persistent-caches.md) per pool |
| Who holds the control plane | Cirun | You |

A software fee and your own compute are different things. Compare Cirun's price
against Zoomies' nothing, but remember both leave you paying for machines, and
that running Zoomies costs you operator time a service would absorb.

## When Cirun is the better choice

- **You do not want to run a controller.** A service takes upgrades, availability
  and the control plane off your plate.
- **You need macOS or a wider platform range today.** Cirun documents VM
  executors for more platforms than Zoomies has qualified.
- **Cloud caching is integrated for you** on the clouds it supports.

## When Zoomies is the better choice

- **You want the control plane on your side.** Your GitHub App, your database and
  your audit log stay on machines you own. See [security](../security.md).
- **You would rather not pay a software fee** per repository, and have machines
  or a home lab already.
- **You want to read and change the code.**

Private hardware is not exclusive to Zoomies; Cirun does it too. The
difference is who runs the controller.

## Questions to put to any managed service

The honest comparison with a service is mostly about what you hand over, so it
is worth asking Cirun, or any service of this kind, the same few questions:

- **Where do the credentials live?** A service that provisions machines in your
  cloud holds a credential for that cloud, and one that registers runners holds
  GitHub access. Find out what each can do and how to revoke it.
- **What happens to queued jobs when the service is unavailable?** With a
  controller you run, that is your uptime to manage and to see. With a service it
  is theirs.
- **What does the fee scale with?** At review it was per private repository, with
  infrastructure extra, so a large estate of small repositories and a small estate
  of busy ones cost very different amounts.

None of this is a mark against a service. It is what the fee buys, set against
what it asks you to trust.

## Moving between them

The two use different labels, so they run side by side. Add a Zoomies pool with
its own label, move one repository's `runs-on` with the
[migration wizard](../migration.md), compare it for a week, then move the rest.
Remove the repositories from Cirun once nothing targets its labels, and the
software fee stops with them.

## Where to go next

- [Quick start](../quickstart.md)
- [What it costs](../costs.md)
- [All alternatives](index.md)
