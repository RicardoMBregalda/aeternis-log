---
title: Pricing & contact
description: Pricing tiers for AeternisLog and how to get in touch about a commercial deployment.
sidebar:
  order: 2
---

import { Card, CardGrid, LinkCard } from '@astrojs/starlight/components';

:::caution[Template pricing]
The tiers below are a **starting template** for commercialization. Replace the
placeholder prices and limits with your final commercial terms before publishing.
:::

## Tiers

<CardGrid>
  <Card title="Community — Free" icon="open-book">
    The full self-hostable platform under an open-source license.

    - Self-hosted, single operator
    - Community support (GitHub issues)
    - All core integrity features

    Best for evaluation and single-tenant deployments.
  </Card>
  <Card title="Team — $—/mo" icon="rocket">
    For teams running a small multi-tenant deployment.

    - Everything in Community
    - Guided per-tenant isolation setup
    - Email support, business-hours
    - Up to _N_ tenants / _N_ anchors per month

    _Set your price and limits._
  </Card>
  <Card title="Enterprise — custom" icon="star">
    For regulated or large-scale deployments.

    - Per-tenant identities & channels, guided
    - Production datastore & multi-org Fabric assistance
    - SSO, security review, SLAs
    - Priority support & roadmap influence

    _Priced per deployment._
  </Card>
</CardGrid>

## Get in touch

AeternisLog is developed in the open. The fastest way to start a conversation —
about Enterprise, a pilot, or a custom deployment — is through GitHub:

<CardGrid>
  <LinkCard
    title="Open an issue"
    href="https://github.com/RicardoMBregalda/aeternis-log/issues/new"
    description="Questions, evaluations, or commercial inquiries."
  />
  <LinkCard
    title="Start a discussion"
    href="https://github.com/RicardoMBregalda/aeternis-log/discussions"
    description="Architecture, use cases, and roadmap."
  />
</CardGrid>

:::tip
Before publishing, swap the GitHub links for a dedicated sales/contact channel
(e.g. a `sales@yourdomain` address or a contact form) if you prefer.
:::
