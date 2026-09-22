# Production audit — 2026-09-22

Read-only checks against https://bible.soh.re and Kubernetes found:

- Six translation-list requests alternated between seven translations and only EMTV/AKJV. Three of six NASB Numbers 1 requests returned 400; three succeeded.
- Replica `bible-soh-re-6fcb7cc599-lpclw` logged failures loading NASB, LEB, KJV, MKJV and BSB (missing `ot.bzv` files). Replica `bible-soh-re-6fcb7cc599-jdljt` downloaded the translation files successfully. Both replicas receive service traffic.
- The deployment runs image `v1.2.7`, has two replicas, no readiness probe, and no volume mounts. Application data is therefore not persisted by this deployment. Inspect database location and backup/recovery requirements before replacing pods; do not blindly restart them.
- All six HTML pages use the Tailwind browser CDN compiler.
- Chapter requests decode error bodies as JSON without checking HTTP status.
- The source uses `session` and `session_id` where authentication actually sets `bible_session`.
- Default navigation assumes Genesis exists, leaving NT-only translations without an initial chapter. Invalid book links also leave the reader empty.
- Concurrent chapter requests can display an older response after newer navigation.

Local changes address these application issues and fail startup when any configured translation cannot load. They have not been deployed. A durable operational fix should provision consistent translation files, add readiness checks, and persist application data before rolling out replacement replicas. The current live image predates the checked-out source, so a release also includes existing unreleased repository changes.

The reported `browser-integration.js` addon-port message appears extension-related; no matching application file exists in this repository. It was not independently reproduced.

This audit covers anonymous reader/API behavior and deployment diagnostics, not authenticated collaboration flows or a complete security review.
