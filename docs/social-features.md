# Social features completed locally

This pass completes the existing private study-group flows: group creation and in-app invitations (including invitations sent before registration), member lists, study-plan creation/editing/deletion and reading links, personal and group notes, threaded comments, reactions, prayer requests, encouragement comments, answered/unanswered status, and archive/restore.

Invitations appear in Study Groups when the recipient signs in with the matching email address. No invitation email or push notification is sent. Public discovery and moderation remain outside this pass.

Access checks cover personal content, group membership, reactions, comment parents, study-plan administration, and authenticated same-origin WebSockets. Live group activity offers a refresh button so an incoming update does not erase a prayer or plan draft. Note refreshes pause while a draft is open.

Validation: Go tests with the race detector cover privacy, cross-group authorization, invitation acceptance, plan validation and mutations, prayer lifecycle, and WebSocket audiences. JavaScript response-parser tests and syntax checks pass. Local browser checks exercised notes, invitations, plan creation and member access, prayer creation/editing, and the mobile prayer form.

The preview runs at http://localhost:8081 using a separate database under tmp/brand-preview. It contains only local demonstration accounts and content. Demo administrator: preview-admin@example.test; password: Local-preview-only-42!.

## Production deployment

Production uses PostgreSQL via `DATABASE_URL`, with two CloudNativePG instances. Local development still supports SQLite when `DATABASE_URL` is unset (requires a CGO-enabled Go build). PostgreSQL LISTEN/NOTIFY carries live updates between application replicas; each WebSocket delivery rechecks session validity and content access.

Two application replicas share a read-only translations PVC and a separate uploads PVC. `/readyz` checks database connectivity and is only served after all configured translations load. Schema initialization is serialized with a PostgreSQL advisory lock. Set `BASE_URL=https://bible.soh.re` to enable secure session cookies.

The previous v1.2.7 production release did not include accounts or a social database, so no production social-data import is required. Local preview accounts are not imported. Email invitations are tracked in [BIBLE-1](https://kanban.standouthost.com/bible/cards/BIBLE-1).
