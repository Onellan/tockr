# Implementation Plan

1. Add config flags for TOTP mode and session workspace behavior.
2. Extend schema additively for TOTP, session workspace, historical/task rates, and user cost rates.
3. Add store methods for profile, password, TOTP, workspaces, project members/groups, calendar queries, and rate history.
4. Add routes and templates:
   - `/account`
   - `/account/password`
   - `/account/totp/enable`
   - `/account/totp/disable`
   - `/calendar`
   - `/workspace`
   - `/projects/{id}/members`
5. Add project row overflow menu because projects now have Dashboard and Members actions.
6. Keep sidebar collapse deferred.
7. Add tests for TOTP, profile/password, workspace switching, memberships, calendar, rate history, and favicon/dropdown regressions.
