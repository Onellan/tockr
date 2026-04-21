# Hierarchy Design

## Summary

Tockr now uses scoped memberships as the source of authorization truth.

The hierarchy is:

- Organization: top-level account boundary.
- Workspace: operational boundary for customers, projects, activities, tags, rates, invoices, reports, webhooks, and members.
- Group: workspace-local team used for bulk assignment and filtering.
- Project: work boundary with direct members and project managers.
- User: belongs to one organization and one or more workspaces.

## Authorization Principle

Permissions are derived from the current request context:

- organization membership role
- current workspace membership role
- project membership role
- group-to-project assignment
- user ownership of records

Flat legacy roles remain only as migration input and compatibility data. Runtime menu visibility and route decisions should use scoped authorization.

## Role Semantics

- Organization Owner: full organization control, workspace creation, global audit/security settings, all workspace access.
- Organization Admin: organization-wide administration without ownership-only destructive actions.
- Workspace Admin: manages workspace members, groups, customers, projects, activities, tags, rates, invoices, webhooks, and workspace reports.
- Workspace Analyst: views workspace-wide reports and read-only operational data.
- Workspace Member: tracks time and views assigned or public workspace data.
- Project Manager: manages assigned project membership and sees project reports/timesheets.
- Project Member: tracks time against assigned project.

## Access Rules

- Users only see workspaces where they have membership, unless they are organization owners/admins.
- Workspace admins see all records in their workspace.
- Analysts see workspace reports but cannot mutate operational data.
- Members see their own timesheets and projects they are assigned to directly or through groups.
- Private projects require direct or group membership.
- Project managers can see project reports and project timesheets for managed projects.
- Sensitive changes produce audit log entries.
