# Martin agent guide

Use this document as the operating contract for an AI agent that installs or
uses Martin. For product context, human setup, examples, and deployment
boundaries, read [`README.md`](README.md) and
[`docs/SECURITY.md`](docs/SECURITY.md). For the persisted event schema, read
[`docs/FACTS.md`](docs/FACTS.md).

## What Martin is

Martin is an opinionated CRM CLI backed by an append-only Jaybase event
history. Humans and agents use the same commands. The domain layer enforces
role-based access, a fixed deal pipeline, the one-next-action rule, uniqueness
constraints, and source-key idempotency before a write is appended.

Martin does not authenticate a human, decide whether source evidence is true,
send email, score leads, or invent pipeline stages. The agent must validate
source evidence, use returned IDs, keep deals on one next action, and obtain
human approval when relationship or commercial judgment is uncertain.

Never read or edit `.martin/`, Jaybase data files, refs, or encryption keys
directly. The CLI is the supported interface.

## Install

Martin requires Go 1.26.5 or later. Earlier Go releases must not be used to
build Martin release binaries. Install it from a repository checkout:

```sh
git clone https://github.com/kyle-visner/martin.git
cd martin
go install ./cmd/martin
martin help
```

For repository-local development, replace `martin` in the examples with:

```sh
go run ./cmd/martin
```

Martin is pre-1.0. Pin the repository revision used by automation and review
release notes before upgrading. `martin version` reports the CLI version string.

## Choose one storage mode

Local mode stores the workspace in a directory:

```sh
martin --store /absolute/path/to/book.martin --actor owner COMMAND...
```

The default local directory is `.martin`. Use an absolute path in unattended
automation so the working directory cannot select the wrong workspace.

Hosted mode uses the authenticated Jaybase API:

```sh
export JAYBASE_URL=https://jaybase.example.com
export JAYBASE_TOKEN='writer-token-from-the-secret-manager'
export MARTIN_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/martin"

martin --actor owner COMMAND...
```

Rules for hosted mode:

- `JAYBASE_URL` must be an HTTPS origin with no credentials, path, query, or
  fragment. Plain HTTP is accepted only for a loopback development server.
- `JAYBASE_TOKEN` is required and is accepted only through the environment.
- Never put the token in a URL, argument, input file, payload, log, prompt
  transcript, or idempotency key.
- Do not combine an explicit `--store` with `JAYBASE_URL` or `--jaybase-url`.
- `MARTIN_CACHE_DIR` / `--cache-dir` is optional and only valid in hosted mode.
  It must be a real private directory, not a symlink, and not group/world
  writable. The checkpoint is disposable; cold replay remains the correctness
  path.
- Use the lowest Jaybase credential role that can complete the task.

Martin's `--actor` is a domain identity, not proof of authentication. Jaybase
authenticates the bearer token but does not prove that it belongs to the actor
named on the command line. Production automation must run behind a trusted
wrapper that binds each authenticated caller to its allowed Martin actor ID.

Global flags must appear before the command:

```text
--store DIR
--jaybase-url HTTPS_ORIGIN
--cache-dir DIR
--actor USER_ID
--role ROLE
```

Omit `--role` normally. If supplied, it must exactly match the role assigned to
the actor; it cannot elevate privileges.

## Initialize and inspect a workspace

Initialize once in the selected storage mode:

```sh
martin --store /absolute/path/to/book.martin --actor owner init --currency USD
```

Initialization establishes the immutable workspace currency. A shared Jaybase
history may already contain Magpie events; Martin adds its own bootstrap and
reads selected Magpie RBAC, customer, and invoice facts while writing only
`martin.*` nodes.

Before any CRM workflow, inspect:

```sh
martin --store /absolute/path/to/book.martin --actor owner doctor
martin --store /absolute/path/to/book.martin --actor owner organization list
martin --store /absolute/path/to/book.martin --actor owner pipeline
```

## Output and error contract

Successful commands write one JSON value to stdout. Parse stdout; do not scrape
human prose. Failures exit nonzero and write one JSON object to stderr:

```json
{"code":"permission_denied","message":"role \"Operations\" lacks required Martin access"}
```

Common codes include:

- `validation_error`: fix the input; do not retry unchanged.
- `permission_denied`: stop and use an authorized actor or credential.
- `not_found`: refresh the relevant ID, root, or snapshot assumption.
- `conflict`: reload state and re-evaluate the operation; never blindly loop.
- `integrity_error`: stop writes and alert an operator.
- `capacity_exceeded`: stop and have an operator restore storage capacity.
- `internal_error`: retry only when the operation has a stable domain identity,
  then alert an operator if it persists.

The CLI has no human-readable output mode. `martin help` prints the top-level
command list; use this guide and the README for command-specific contracts.

## Non-negotiable CRM rules

- Represent money as integer minor units (`value-cents`). Never use
  floating-point dollars.
- Use `YYYY-MM-DD` for business dates and RFC3339 for activity timestamps.
- Use returned opaque IDs such as `org:...`, `person:...`, `deal:...`, and
  `task:...`; never construct or guess them.
- Mutations select records by ID, never fuzzy names.
- Pipeline stages are exactly: `new`, `qualified`, `proposal`, `won`, `lost`.
- Every open deal has exactly one pending next action.
- `deal create` requires `--next-action` and `--next-due`.
- `deal advance` moves one open stage and replaces the next action.
- Proposal-stage deals must be won or lost; do not attempt another advance.
- `deal touch` is the preferred path when logging an interaction and setting the
  next step in one atomic write.
- Generic `task create` is for relationship follow-up only. Do not attach
  generic tasks to deals.
- Records are archived, canceled, merged, or superseded. There is no delete.
- Emails are unique across active people, case-insensitively.
- Organization domains are unique across active organizations.
- Customer links are explicit and one-to-one. Never fuzzy-match Magpie
  customers by name.
- Martin never writes Magpie accounting facts. Magpie remains authoritative for
  customers, invoices, and the ledger.
- Before a large or risky workflow, create a named snapshot. A Martin snapshot
  is a root checkpoint, not an off-host backup.
- Corrections preserve history. Prefer new activities, task cancellations, deal
  reopen with reason, or merges over inventing destructive edits.

## Safe daily workflow

For ordinary CRM work:

1. Run `doctor` or `today` to understand the current root and due work.
2. Search or list by known IDs; do not invent identifiers.
3. Create or update organizations and people only with verified contact facts.
4. Create deals only when the organization/person IDs, value, expected close,
   and first next action are known.
5. Prefer `deal touch` after calls, emails, and meetings.
6. Advance only when the stage change is real; win or lose from proposal with a
   close date and, for losses, a reason.
7. Log standalone relationship activities when they are not part of a deal
   touch.
8. Parse the returned JSON and retain returned IDs and root.
9. On an ambiguous transport result, re-read before retrying. Reuse the same
   import `(source, source_key)` or natural domain identity; never invent a new
   identity to force a duplicate write.

Recommended loop for an agent operator:

```sh
martin --actor AGENT_USER_ID today
martin --actor AGENT_USER_ID pipeline
martin --actor AGENT_USER_ID search --query "acme"
martin --actor AGENT_USER_ID deal get --id deal:...
```

## Import workflow

`import-json` requires `manage` access and a normalized bundle:

```json
{
  "source": "legacy-crm",
  "source_key": "export-2026-08-01",
  "organizations": [],
  "people": [],
  "deals": [],
  "activities": [],
  "tasks": [],
  "customer_links": []
}
```

Rules:

- `source` and `source_key` are required.
- Replaying the same content returns the current root without appending.
- Reusing the key for different content is a conflict.
- Open imported deals require one matching pending next task.
- Imported entities must include stable IDs and satisfy ordinary domain
  validation against the combined pre- and post-import projection.
- Prefer small reviewed bundles over giant unattended dumps.

Example:

```sh
martin --actor owner import-json --file ./normalized-import.json
```

`--file -` reads stdin. Keep import files out of git if they contain personal
data.

## Magpie bridge

Martin may read:

- Magpie users and roles for access control;
- Magpie customers for link targets and combined views;
- Magpie invoices for read-only customer context.

Martin writes only:

- `martin.*` facts, including `martin.customer-link.*`.

To connect CRM and accounting:

```sh
martin --actor owner customer link \
  --organization-id org:... \
  --magpie-customer-id cust:...
```

Exactly one of `--organization-id` or `--person-id` is used with
`--magpie-customer-id`. Unlink requires a reason. Do not attempt to create or
edit Magpie customers from Martin.

## Access model

| Magpie role | Martin access |
| --- | --- |
| Owner, Admin | read, write, manage |
| Sales Rep | read, write |
| Operations, Accountant | read |

`manage` is required for merges, reopening closed deals, bulk imports, customer
links, and snapshots. On a Martin-first standalone store, only actor `owner` is
accepted until Magpie initializes shared users.

`state`, `export`, and `audit` expose broad reconstructed or historical data.
Give those commands only to actors that need them.

## Command map

```text
init [--currency USD]
doctor
today [--owner ID] [--as-of YYYY-MM-DD]
pipeline [--owner ID]
search --query TEXT
state
export
audit
import-json --file FILE
snapshot create --name NAME
version

organization create --name NAME [--domain DOMAIN] [--email EMAIL] [--phone PHONE] [--owner ID] [--tags a,b] [--id ID]
organization update --id ID [fields]
organization get --id ID
organization list [--include-archived]
organization archive --id ID --reason REASON
organization merge --from ID --into ID --reason REASON

person create --display-name NAME [--organization-id ID] [--email EMAIL] [--phone PHONE] [--title TITLE] [--owner ID] [--tags a,b] [--id ID]
person update --id ID [fields]
person get --id ID
person list [--include-archived]
person archive --id ID --reason REASON
person merge --from ID --into ID --reason REASON

deal create --name NAME [--organization-id ID] [--person-id ID] [--owner ID] --value-cents N --expected-close YYYY-MM-DD --next-action TEXT --next-due YYYY-MM-DD [--id ID]
deal advance --id ID --next-action TEXT --next-due YYYY-MM-DD
deal touch --id ID --kind call|email|meeting|note --summary TEXT [--occurred-at RFC3339] --next-action TEXT --next-due YYYY-MM-DD
deal win --id ID --closed-on YYYY-MM-DD
deal lose --id ID --closed-on YYYY-MM-DD --reason REASON
deal reopen --id ID --reason REASON --next-action TEXT --next-due YYYY-MM-DD
deal get --id ID
deal list [--include-closed]

activity log --kind call|email|meeting|note --summary TEXT [--occurred-at RFC3339] [--organization-id ID] [--person-id ID] [--deal-id ID]
activity list [--organization-id ID] [--person-id ID] [--deal-id ID]

task create --title TEXT --due YYYY-MM-DD [--owner ID] [--organization-id ID] [--person-id ID]
task complete --id ID
task cancel --id ID --reason REASON
task list [--owner ID] [--status pending|completed|canceled]

customer link [--organization-id ID|--person-id ID] --magpie-customer-id ID
customer unlink --magpie-customer-id ID --reason REASON
customer get --magpie-customer-id ID
customer list [--include-removed]
```

## Security boundaries

- Treat local store contents, decrypted JSON output, import files, and hosted
  responses as sensitive CRM data containing personal contact information.
- Keep local stores and keys on encrypted disks with restrictive filesystem
  permissions. For production local storage, provide `JAYBASE_DATA_KEY` through
  managed secret delivery and separate keys from backups.
- Keep hosted bearer tokens and Jaybase data keys in a secret manager.
- Keep `MARTIN_CACHE_DIR` private. Do not copy checkpoints between machines or
  users.
- Use opaque IDs in event metadata; payloads are encrypted, but event type,
  entity ID, actor, role, command, timestamp, and history shape are metadata.
- Do not store credentials, tax identifiers, payment card data, or other secrets
  in CRM fields merely because payload encryption is available.
- Copy backups or snapshots off-host and test restoration. A successful command
  is not proof that disaster recovery works.
- Stop on integrity failures. Never bypass a failed read or weaken validation to
  make a CRM write succeed.

## Agent completion checklist

Before reporting success, verify:

- the command exited zero and stdout parsed as JSON;
- the returned actor, entity IDs, stage, next action, and root match the
  intended workspace;
- open deals still have exactly one pending next action;
- money values remained integer minor units;
- durable import source keys or natural IDs were retained for replay safety;
- no secret appeared in arguments, files committed to source control, logs, or
  the report;
- any Magpie customer claim distinguishes an explicit Martin link from a
  guessed name match;
- any snapshot or backup claim distinguishes a local checkpoint from an
  independently stored and restore-tested backup.
