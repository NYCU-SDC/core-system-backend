# Organization Lifecycle Test Scenarios

This directory contains automated QA tests for organization creation and deletion in the Core System API. It is used to set up an **organization where the current user is not a member**, for negative tests (e.g. `../negative-form-lifecycle/02a-form-creation-org-membership-negative.http` — create form under org → 403).

## Overview

These tests cover:

- **Creation** – Create an org with a fixed slug (`org-403-test`) and export `orgSlugNotMember` / `newOrgId` for use in negative-form-lifecycle.
- **Cleanup** – Re-add the current user as a member, then delete the org.

The flow leaves the user **not** a member of the org after creation so that other scenarios can assert 403 when acting on that org.

## Test Files

Execute tests in the following order:

1. **`01-organization-creation.http`** – Create Org for Negative Tests
   - Uses shared authentication from `../shared/common.http`
   - GET `/users/me` for user info
   - POST `/orgs` to create org (name: "Org For 403 Test", slug: `org-403-test`)
   - If org already exists, GET `/orgs/org-403-test` to obtain slug/id
   - Exports: `orgSlugNotMember`, `newOrgId`

2. **`02-organization-deletion.http`** – Cleanup & Delete Org
   - Imports `./01-organization-creation.http` for variables
   - Verifies org exists (GET org)
   - GET `/users/me` for `userEmail`
   - POST `/orgs/{{orgSlugNotMember}}/members` to add user back
   - DELETE `/orgs/{{orgSlugNotMember}}` to remove the org

## Setup

1. Copy the environment file:

   ```bash
   cp .env.example .env
   ```

2. Edit `.env` with your configuration:

   ```env
   BASE_URL=http://127.0.0.1:4010
   LOGIN_USER_ID=<your-user-uuid>
   ```

3. Ensure the API server is running at the specified `BASE_URL`.

4. **Note:** `LOGIN_USER_ID` must be a valid user UUID in your system; the placeholder in `.env.example` will not work.

## Running Tests

### Run Single Test File

```bash
httpyac send scenarios/user-journey/organization-lifecycle/01-organization-creation.http
```

### Run with Specific Environment

```bash
httpyac send scenarios/user-journey/organization-lifecycle/*.http --env dev
```

## Dependencies

- **Shared auth** – `../shared/common.http` for authentication
- **httpyac** – HTTP test runner
- **Node.js** – For assertions in `??` blocks
- Running Core System API instance

## Relationship to Other Scenarios

- **negative-form-lifecycle** – `02a-form-creation-org-membership-negative.http` imports `01-organization-creation.http` to get `orgSlugNotMember` and test creating a form under an org where the user is not a member (expect 403).

## Troubleshooting

### Authentication Failures

- Verify `LOGIN_USER_ID` in `.env` is a valid user UUID (not the placeholder).
- Ensure internal login is enabled (e.g. debug mode).

### 404 "User not found"

- `LOGIN_USER_ID` is invalid; use a real user UUID from your system.

### Org already exists

- `01-organization-creation.http` handles it by GETting the org and exporting the same variables; no change needed.

## Contributing

When adding or changing test files:

1. Follow the naming convention: `NN-description.http`.
2. Use `@import` from `../shared/common.http` or the previous file as needed.
3. Export variables used by dependent scenarios (e.g. `orgSlugNotMember`).
4. Update this README with new steps and descriptions.
