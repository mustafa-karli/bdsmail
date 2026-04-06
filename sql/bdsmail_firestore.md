# BDS Mail - Firestore Setup

Firestore collections are created automatically on first document write.
No explicit DDL is needed. However, the Firestore database must exist.

## Prerequisites

```bash
# Create Firestore database (one-time)
gcloud firestore databases create --location=us-west1
```

## Collections

| Collection | Document ID | Notes |
|-----------|------------|-------|
| bdsmail-users | email (e.g. alice@domain.com) | User accounts |
| bdsmail-messages | auto-generated UUID | Partitioned by owner_user |
| bdsmail-aliases | alias_email | Email forwarding rules |
| bdsmail-mailing-lists | list_address | Group distribution lists |
| bdsmail-list-members | list_address_member_email | Composite key |
| bdsmail-filters | auto-generated UUID | Per-user mail filters |
| bdsmail-auto-replies | user_email | Vacation/auto-reply config |
| bdsmail-auto-reply-log | user_email_sender_email | Cooldown tracking |
| bdsmail-contacts | auto-generated UUID | CardDAV contacts |
| bdsmail-domains | domain name | Registered domains |
| bdsmail-oauth-clients | auto-generated UUID | OAuth app registrations |
| bdsmail-oauth-codes | code | Short-lived auth codes |
| bdsmail-oauth-tokens | token | Access tokens |

## Composite Indexes

Firestore requires composite indexes for queries that combine `Where` with `OrderBy`.
Create these in the Firebase Console or via `gcloud`:

```bash
# Messages: query by owner + folder, order by received_at
gcloud firestore indexes composite create \
  --collection-group=bdsmail-messages \
  --field-config field-path=owner_user,order=ASCENDING \
  --field-config field-path=folder,order=ASCENDING \
  --field-config field-path=deleted,order=ASCENDING \
  --field-config field-path=received_at,order=DESCENDING

# OAuth clients: query by domain
gcloud firestore indexes composite create \
  --collection-group=bdsmail-oauth-clients \
  --field-config field-path=domain,order=ASCENDING \
  --field-config field-path=created_at,order=DESCENDING

# Contacts: query by owner_email
gcloud firestore indexes composite create \
  --collection-group=bdsmail-contacts \
  --field-config field-path=owner_email,order=ASCENDING \
  --field-config field-path=updated_at,order=DESCENDING
```

Alternatively, Firestore will suggest missing indexes in error messages at runtime. Follow the links in the error to create them automatically.
