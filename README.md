# Martin

Martin is an opinionated command-line CRM for small businesses. Humans and
agents use the same JSON interface, and Martin records immutable CRM facts in
Jaybase.

Martin is intentionally small:

- organizations and people;
- one sales pipeline: `new -> qualified -> proposal -> won|lost`;
- calls, emails, meetings, and notes;
- relationship follow-up tasks;
- explicit links to Magpie accounting customers;
- daily work and pipeline reports;
- idempotent normalized JSON imports.

It does not include marketing automation, email sending, support tickets,
product catalogs, arbitrary workflow builders, or predictive lead scoring.

## The operating rule

Every open deal has exactly one pending next action.

- `deal create` requires the first next action.
- `deal advance` completes the old action and creates the next one.
- `deal touch` atomically records an interaction, completes the old action,
  and creates the next one.
- `deal win` and `deal lose` close the outstanding action.

Generic tasks cannot be attached to deals. This keeps deal work on the one
canonical workflow.

## Build and verify

Martin depends on the private Jaybase Go module:

```sh
export GOPRIVATE=github.com/kyle-visner/*

go test ./...
go test -race ./...
go vet ./...
go build -o ./martin ./cmd/martin
```

If the default Go build cache is not writable:

```sh
GOCACHE=/private/tmp/martin-go-cache go test ./...
```

## Shared Jaybase history

Martin is designed to run against the same Jaybase instance as Magpie.

```sh
export JAYBASE_URL=https://jaybase.example.com
export JAYBASE_TOKEN='writer-token-from-the-secret-manager'
export MARTIN_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/martin"

./martin --actor owner init --currency USD
```

The token is accepted only through `JAYBASE_TOKEN`. Do not put it in a command
argument, URL, payload, log, or idempotency key.

`MARTIN_CACHE_DIR` is optional. When set, Martin stores an encrypted,
token-bound projection checkpoint there and incrementally replays only newer
facts. The checkpoint is a disposable performance aid, not a source of truth
or a backup. Keep the directory private and do not share it across untrusted
users.

Martin writes only `martin.*` node types. Magpie `main` at and after merge
`e8fc9d7` skips that namespace while advancing across the shared root. Martin
reads selected Magpie RBAC, customer, and invoice facts to provide a combined
customer view, but it never writes Magpie accounting facts.

Customer links are explicit and one-to-one:

```sh
./martin --actor owner customer link \
  --organization-id org:... \
  --magpie-customer-id cust:...
```

Martin does not fuzzy-match or silently synchronize names. Martin remains
authoritative for CRM facts; Magpie remains authoritative for accounting facts.

## Local development

Martin can also use an embedded local Jaybase store:

```sh
go run ./cmd/martin --store .martin --actor owner init --currency USD
```

A Martin-first store remains compatible with later Magpie initialization
because Magpie initialization is domain-aware.

## Typical workflow

```sh
./martin --store .martin --actor owner organization create \
  --name "Acme Studio" \
  --domain acme.example \
  --tags prospect,design

./martin --store .martin --actor owner person create \
  --display-name "Ada Lovelace" \
  --organization-id org:... \
  --email ada@acme.example

./martin --store .martin --actor owner deal create \
  --name "Website redesign" \
  --organization-id org:... \
  --person-id person:... \
  --value-cents 1250000 \
  --expected-close 2026-08-31 \
  --next-action "Run discovery call" \
  --next-due 2026-07-25

./martin --store .martin --actor owner deal touch \
  --id deal:... \
  --kind meeting \
  --summary "Discovery completed; scope confirmed" \
  --next-action "Send proposal" \
  --next-due 2026-07-28

./martin --store .martin --actor owner deal advance \
  --id deal:... \
  --next-action "Review proposal" \
  --next-due 2026-07-30

./martin --store .martin --actor owner today
./martin --store .martin --actor owner pipeline
```

All successful command output is JSON on stdout. Errors are JSON on stderr:

```json
{"code":"validation_error","message":"next action is required"}
```

## Access model

Martin reuses Magpie users and role names when they exist:

| Magpie role | Martin access |
| --- | --- |
| Owner, Admin | read, write, manage |
| Sales Rep | read, write |
| Operations, Accountant | read |

`manage` is required for record merges, reopening closed deals, bulk imports,
customer links, and snapshots. On a Martin-first standalone store, only the
`owner` actor is accepted until Magpie initializes shared users.

## Data rules

- Money is integer minor units in the workspace's immutable ISO currency.
- Business dates use `YYYY-MM-DD`; activity timestamps use RFC3339.
- Mutations select records by ID, never fuzzy names.
- Records are archived, canceled, merged, or superseded, never deleted.
- Emails are unique across active people, case-insensitively.
- Organization domains are unique across active organizations.
- Bulk imports require `(source, source_key)` and are replay-safe.
- Audit output is metadata-only.
- Snapshot refs use the `martin-` prefix and protect the complete shared root.

See [docs/FACTS.md](docs/FACTS.md) for the persisted event contract.

## License

AGPL-3.0-or-later. See `LICENSE`.
