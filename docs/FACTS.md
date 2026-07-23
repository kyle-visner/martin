# Martin fact contract

Martin stores immutable JSON facts in Jaybase. The Jaybase node `type` is the
schema identifier and is always prefixed with `martin.`.

## Event types

| Type | Projection effect |
| --- | --- |
| `martin.workspace.initialized.v1` | Establish the immutable workspace currency |
| `martin.organization.created.v1` | Create an organization |
| `martin.organization.updated.v1` | Replace the current organization projection |
| `martin.organization.archived.v1` | Archive an organization |
| `martin.organization.merged.v1` | Redirect relationships to a surviving organization |
| `martin.person.created.v1` | Create a person |
| `martin.person.updated.v1` | Replace the current person projection |
| `martin.person.archived.v1` | Archive a person |
| `martin.person.merged.v1` | Redirect relationships to a surviving person |
| `martin.deal.created.v1` | Create a new deal and its first next action |
| `martin.deal.advanced.v1` | Move one stage and replace the next action |
| `martin.deal.touched.v1` | Log an activity and replace the next action atomically |
| `martin.deal.won.v1` | Win a deal and close its next action |
| `martin.deal.lost.v1` | Lose a deal and close its next action |
| `martin.deal.reopened.v1` | Reopen a closed deal with an audit reason and next action |
| `martin.activity.logged.v1` | Add an immutable interaction |
| `martin.task.created.v1` | Add a relationship follow-up task |
| `martin.task.completed.v1` | Complete a non-deal task |
| `martin.task.canceled.v1` | Cancel a non-deal task with a reason |
| `martin.customer-link.created.v1` | Link a CRM entity to a Magpie customer |
| `martin.customer-link.removed.v1` | Remove a customer link with a reason |
| `martin.import.applied.v1` | Apply one normalized, idempotent import bundle |

Unknown `martin.*` event types fail closed during replay. Other application
namespaces are ignored while Martin still advances its state root across them.

## Magpie bridge

Martin reads these legacy Magpie node types:

- `store.init` for users and roles;
- `rbac.role` and `rbac.user` for subsequent access changes;
- `customer` for current accounting-customer identity;
- `invoice` for read-only invoice context.

Other Magpie facts are ignored by Martin. Martin never emits a legacy Magpie
node type.

Customer links use `martin.customer-link.*`, not `shared.*`, because the merged
Magpie compatibility contract currently recognizes `martin.*` as the foreign
namespace. Moving links to a shared namespace requires a separately versioned
Magpie compatibility change.

## Concurrency and idempotency

Each write:

1. captures and projects the complete current Jaybase root;
2. validates the domain command against that projection;
3. appends with Jaybase `expected_root`;
4. returns a conflict rather than overwriting a concurrent fact.

Hosted requests use a content-derived `Idempotency-Key` prefixed with
`martin-`. GET/HEAD and idempotent writes may retry transient transport errors.
Named-ref writes are not automatically retried; Martin reconciles their durable
value after an ambiguous outcome.

Imports add a domain-level uniqueness check on `(source, source_key)`. Replaying
the same content returns the current root without appending. Reusing the key for
different content is a conflict.

## Hosted projection checkpoint

When `MARTIN_CACHE_DIR` or `--cache-dir` is set, Martin may save an encrypted
projection checkpoint and request only events after its recorded root. The
checkpoint is bound to the normalized Jaybase origin and token, contains no
authoritative facts, and is discarded automatically if it is corrupt or its
root is no longer present. Cold replay from Jaybase remains the correctness
path.
