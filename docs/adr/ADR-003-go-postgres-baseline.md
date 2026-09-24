# ADR-003 — Go and PostgreSQL Driver Baseline

Date: 2026-09-24
Status: **ACCEPTED**

## Context

engineering-platform is a clean-slate implementation and has no compatibility requirement with the original Go 1.23 bootstrap.

The first production persistence backend uses PostgreSQL and needs:
- native transaction control;
- PostgreSQL error semantics;
- connection pooling;
- context-aware operations;
- a maintained driver.

As of 2026-09-24, pgx v5 is the stable major line, pgx v5.11.0 is current, and its minimum supported Go version is Go 1.25.

## Decision

- minimum Go version: **1.25**;
- PostgreSQL driver/toolkit: **github.com/jackc/pgx/v5 v5.11.0**;
- use pgx native transactions rather than adding a generic ORM;
- keep SQL and authority transaction semantics explicit;
- PostgreSQL integration tests run in CI against a real PostgreSQL service.

## Why not preserve Go 1.23

There is no deployed compatibility contract yet.

Pinning an older database driver only to preserve the bootstrap language version would create immediate compatibility/security/maintenance debt without user value.

## Why no ORM

The critical persistence requirements are transaction semantics, optimistic concurrency, audit/outbox atomicity, row locking, and exact SQL effects.

An ORM would not remove those design requirements and would make the authority transaction boundary less explicit.

## Dependency policy

- stay on pgx v5 stable releases;
- upgrades require CI integration tests;
- Go minimum may increase when the supported pgx line requires it;
- dependency changes never alter domain authority semantics.
