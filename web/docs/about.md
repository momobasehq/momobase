---
sidebar: false
aside: false
---

# About Momobase

Momobase is an open-source, self-hosted payment orchestration service. It gives applications one API for collections and disbursements while provider adapters handle credentials, request formats, webhooks, and status mapping.

## Why Momobase exists

Payment providers expose different contracts and operational behavior. Supporting each provider directly spreads payment logic across applications and makes routing, retries, and reconciliation difficult to operate consistently.

Momobase keeps that complexity in one service. Applications use a stable payment contract while operators control provider accounts, routing rules, transaction data, and deployment infrastructure.

## What Momobase handles

- Application credentials, administrator roles, and permissions.
- Idempotent collections and disbursements.
- Provider accounts, encrypted configuration, health, and routing.
- Normalized transaction state, webhooks, and reconciliation.
- Audit records, an Admin API, and an optional embedded dashboard.

## What stays outside Momobase

Momobase is not a payment provider, merchant of record, or order-management system. You keep the commercial relationship with each provider, and your application remains the source of truth for orders and customer-facing business state.

## How you can run it

Use the supplied `momobase` binary as a standalone service, or import the root Go package and register the providers your application needs. The supplied binary includes the dummy provider for testing and moves no money.

The [Momobase guide](/guide/) explains the runtime model, local setup, deployment, extensions, provider adapters, and package integration.

::: warning Project status
Momobase is early-stage software. Review its configuration, provider adapters, operational controls, and failure behavior before using it for production money movement.
:::
