# Security Controls

This document records the controls intended to support SOC 2 and similar
governance programs for Martin. It is not a certification claim.

## Implemented Controls

- Least-privilege access is enforced before CRM reads and writes by reusing
  Magpie users and role names when present.
- All business mutations are immutable `martin.*` events in Jaybase's
  content-addressed history.
- Hosted Jaybase access requires a bearer token and HTTPS outside loopback
  development.
- Hosted appends use optimistic root preconditions and content-derived
  `martin-` idempotency keys so concurrent changes and ambiguous retries cannot
  silently duplicate or overwrite events.
- Hosted replay follows the authenticated, payload-explicit, paginated
  `/v1/events` API rather than reading Jaybase data files directly.
- Optional hosted projection checkpoints are AES-256-GCM encrypted, bound to
  the normalized origin and token, stored only under a private directory, and
  discarded automatically when corrupt or orphaned.
- Domain mutations select records by opaque ID, not fuzzy name matching.
- Records are archived, canceled, merged, or superseded; there is no delete
  path for CRM history.
- Bulk imports require `(source, source_key)` and are digest-idempotent.
- Audit output is metadata-only.
- Snapshot refs use the `martin-` prefix and protect the complete shared root.
- Pull requests and tagged releases run tests, race detection, vet, module
  verification, and pinned `govulncheck` scanning with Go 1.26.5.

## Access model

| Magpie role | Martin access |
| --- | --- |
| Owner, Admin | read, write, manage |
| Sales Rep | read, write |
| Operations, Accountant | read |

`manage` is required for merges, reopening closed deals, bulk imports,
customer links, and snapshots. On a Martin-first standalone store, only the
`owner` actor is accepted until Magpie initializes shared users.

Martin's `--actor` is a domain identity, not proof of authentication. Jaybase
authenticates the bearer token but does not prove that it belongs to the actor
named on the command line.

## Trust model

Martin, Magpie, and Jaybase are single-tenant systems: one organization, one
trust boundary, and one operator (often a trusted AI chat acting for an admin
user). They are not multi-tenant SaaS. The CLI `--actor` flag is appropriate
in that model because the human admin already owns the host, the Jaybase
token, and the agent session.

Hardening that matters only when the trust boundary expands (shared hosts,
multiple untrusted operators, customer-hosted multi-tenant deploys) is listed
separately below.

## Single-tenant operator checklist

- Provide Jaybase data keys and bearer tokens from a secret manager or other
  private store. Never put `JAYBASE_TOKEN` in a flag, URL, payload, log,
  prompt transcript, or idempotency key.
- Keep `MARTIN_CACHE_DIR` private to the operator account. Do not share
  checkpoints across machines or users.
- Treat decrypted JSON output, local stores, import bundles, and hosted
  responses as sensitive CRM contact data for that one tenant.
- Do not store credentials, tax identifiers, payment card data, or other
  secrets in CRM fields merely because Jaybase encrypts payloads.
- Keep ordinary off-host backups of the Jaybase volume/keys. A Martin snapshot
  is a named root checkpoint, not a backup.
- Prefer the lowest Jaybase token role that can complete the task.

## Later, if the trust boundary expands

These are not blockers for the current single-tenant admin + trusted-agent
shape:

- Bind authenticated principals to allowed Martin actor identities instead of
  unrestricted local CLI actor flags.
- Signed command envelopes for non-interactive agents on shared hosts.
- Centralized audit export with retention policies.
- Formal multi-tenant data-classification and isolation rules.
- SBOM generation in the release workflow.
- Actor-binding and stronger tenancy controls for customer-hosted multi-user
  deployments.
