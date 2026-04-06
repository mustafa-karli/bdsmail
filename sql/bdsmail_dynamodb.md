# BDS Mail - DynamoDB Table Definitions

Tables are created automatically by the application on first startup.
This document describes the table structures for reference.

## Tables

| Table | Partition Key | Sort Key | GSI |
|-------|--------------|----------|-----|
| bdsmail-users | email (S) | - | - |
| bdsmail-messages | owner_user (S) | sort_key (S) | folder-index (owner_user, folder), id-index (id) |
| bdsmail-aliases | alias_email (S) | - | - |
| bdsmail-mailing-lists | list_address (S) | - | - |
| bdsmail-list-members | list_address (S) | member_email (S) | - |
| bdsmail-filters | user_email (S) | id (S) | - |
| bdsmail-auto-replies | user_email (S) | - | - |
| bdsmail-auto-reply-log | user_email (S) | sender_email (S) | - |
| bdsmail-contacts | owner_email (S) | id (S) | - |
| bdsmail-domains | name (S) | - | - |
| bdsmail-oauth-clients | domain (S) | id (S) | client-id-index (client_id) |
| bdsmail-oauth-codes | code (S) | - | - |
| bdsmail-oauth-tokens | token (S) | - | - |

All tables use PAY_PER_REQUEST billing mode (on-demand, no provisioned capacity).
