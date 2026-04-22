# Security Impact

## TOTP

- Controlled by `TOCKR_TOTP_MODE`: `disabled`, `optional`, or `required`.
- Users with TOTP enabled must provide a valid TOTP code or recovery code at login.
- Required mode forces users without TOTP to complete setup before normal app access.
- TOTP secrets are stored server-side and never shown after setup.
- Recovery codes are shown once and stored as password hashes.

## Account Settings

- Users can update only their own display name/timezone/password/TOTP settings.
- Email remains admin-controlled for now.
- Password changes require the current password.

## Workspace Switching

- Switch requests verify membership or organization-admin access server-side.
- Session workspace controls request access context.

## Project Membership

- Only workspace admins and project managers can edit memberships.
- Project manager cannot grant workspace-admin powers.
- All changes are audit logged.
