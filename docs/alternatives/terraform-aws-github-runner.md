---
icon: material/aws
title: "Zoomies vs terraform-aws-github-runner"
description: >-
  How Zoomies compares with the AWS Terraform module for self-hosted runners: an
  infrastructure-as-code EC2 autoscaler against a host-based fleet controller.
---

# Zoomies and the AWS Terraform module

[terraform-aws-github-runner](https://github.com/github-aws-runners/terraform-aws-github-runner)
is a Terraform module that builds event-driven, auto-scaling GitHub Actions
runners on EC2 in your own AWS account. It is the maintained successor to the
Philips Labs repository, which is archived. If your infrastructure is AWS and
Terraform, it is a strong choice, and this page is about when it is and when
Zoomies is.

Written from the module's public documentation, reviewed in October 2026.

## Side by side

| | AWS Terraform module | Zoomies |
| --- | --- | --- |
| Model and licence | Terraform module you apply and operate; MIT | One Go binary you run; [AGPL-3.0](https://github.com/eyupio/zoomies/blob/main/LICENSE) |
| Runs on | EC2, including spot, in your AWS account | Linux hosts anywhere an agent can dial out from |
| Scaling | Lambda functions react to webhooks and scale to zero | A controller reconciles pools against queued jobs, and scales to zero by default |
| Images | Your own AMIs and subnets | [Runner images](../naming.md#the-runner-image) for Ubuntu, Debian, Fedora and Rocky Linux |
| Architectures | Linux x64 and arm64, and Windows | Linux x64 and arm64; Windows [not yet qualified](../index.md#what-is-qualified) |
| State | Your AWS account and Terraform state | One SQLite file |
| Seeing the fleet | Your AWS tooling | A live [web UI](../ui.md), [metrics](../metrics.md) and an audit log |
| Runner levels | Organisation and repository; enterprise level is not supported yet | Repository and organisation; [enterprise level is not supported](../many-repositories.md) |
| GitHub Enterprise Server | Supported, along with GitHub Cloud with data residency | Configurable, [not yet tested against one](../faq.md#does-it-work-with-github-enterprise-server) |
| Several runner shapes | Multi-runner: several configurations from one deployment | Several [pools](../hosts-and-pools.md), each with its own labels, image and size |

## When the AWS module is the better choice

- **Everything you run is on AWS.** CI sits in the same account, network, IAM and
  bill as the rest, and the module is infrastructure as code like the rest.
- **You want a fresh machine for a job**, and spot pricing for it. Zoomies gives
  each job a fresh container on a shared host instead.
- **You need Windows runners today.** The module documents them.

## When Zoomies is the better choice

- **You are not all on one cloud**, or not in one at all. Hosts can be a rack, an
  office, a home lab, a Proxmox cluster and a rented server in one fleet.
- **You do not want a Terraform pipeline to run CI.** One command installs the
  controller; a host joins with one more. See the [quick start](../quickstart.md).
- **You are moving repositories off hosted runners.** The
  [migration wizard](../migration.md) rewrites `runs-on` across them.

## What setting it up involves

The module's documentation lists the steps: set up your AWS account, create and
configure a GitHub App, download or build the Lambda functions, deploy the module
with Terraform, then install the App and add repositories to the runner groups.
That is a sensible path if you already have a Terraform pipeline and an AWS
account laid out for it, and a long one if you do not.

Zoomies has fewer parts and fewer places they can live. The
[installer](../quickstart.md) puts one binary on a host, a dialog creates the
GitHub App, and a pool says what a runner looks like. The cost is that the
machines are yours to provide: Zoomies does not rent EC2 for you.

## How the cost behaves

The module's own description is that it runs on spot instances, creates a runner
on demand, ends it after the job and scales to zero when no workflows are active.
You pay AWS for the time a runner exists, and nothing for idle capacity.

Hosts you own or rent by the month cost the same whether busy or idle, but a job
on them costs nothing extra once the host is paid for. Which is cheaper depends
on how busy the machines would be. [What it costs](../costs.md) has the sums, and
the section on working out whether self-hosting pays is the one to read.

## Moving between them

The two use different labels, so they run side by side. A sensible order:

1. Add a Zoomies pool with its own label, and a host for it to use.
2. Move one repository: the [migration wizard](../migration.md) rewrites its
   `runs-on` and opens a pull request.
3. Watch that repository's jobs for a week on both sides of the comparison that
   matters to you: queue time, duration and cost.
4. Move the rest, one repository at a time.
5. Remove the repositories from the module's runner groups, and destroy its
   Terraform stack once nothing targets it.

Moving back is the same steps in reverse, because neither tool changes your
workflow files beyond the `runs-on` line.

## Where to go next

- [Hosts and pools](../hosts-and-pools.md): how Zoomies decides where a runner
  goes.
- [What it costs](../costs.md): the sums for self-hosting against hosted runners.
- [All alternatives](index.md)
