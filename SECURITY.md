# Security policy

## Supported versions

Only the latest released version receives security fixes.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub's
[security advisories](https://github.com/matiasrohleder/PostDream/security/advisories/new) ("Report a vulnerability").
Include the affected version, a description, reproduction steps and the impact you expect.

You will get an acknowledgement within a few days. Fixes are released as a new patch version and credited in the
release notes unless you prefer otherwise.

## Security model (summary)

- An **inbox is the tenant**: API tokens only see their inbox; member users only see their assigned inboxes.
- Email HTML is rendered in a sandboxed iframe without scripts; attachments are always downloaded, never rendered.
- Outbound requests to user-supplied URLs (webhooks, link checks) cannot reach private networks unless an admin allows
  the host; the address is validated after DNS resolution.
- Nothing is relayed to real recipients unless `RELAY_SMTP_URL` is set **and** the inbox has a recipient allow-list;
  every release is audited.
- Inbox SMTP passwords are stored in plain text on purpose (the UI shows them); use sandbox-only credentials.
