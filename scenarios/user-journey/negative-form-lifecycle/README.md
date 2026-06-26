# Negative Form Lifecycle Test Scenarios

This directory contains automated QA tests for **error paths** of the form lifecycle in the Core System API. These tests validate that invalid inputs, missing resources, and forbidden operations return the expected HTTP status and error payloads.

## Overview

Negative tests cover:

- **404** – Not Found (invalid slugs, non-existent form/section/question/response IDs)
- **400** – Validation / Bad Request (missing required fields, invalid types, invalid values)
- **403** – Forbidden (e.g. creating a form under an org the user is not a member of)
- Highlight error paths for invalid question types and missing highlight resources

These scenarios **depend on the positive form lifecycle** (`../form-lifecycle/`). Each negative file imports the corresponding positive flow to get auth, org, form, section, question, and response variables, then runs only the negative cases.

## Test Files

Execute tests in the following order:

1. **`01-setup-negative.http`** – Setup & Auth Error Paths
   - Uses shared auth from `../shared/common.http`
   - Expects positive setup (org, verifyOrg) to run first for variables
   - Invalid org slug → 404

2. **`02-form-creation-negative.http`** – Form CRUD Error Paths
   - Imports `../form-lifecycle/01-setup.http` and `../form-lifecycle/02-form-creation.http`
   - Missing title / invalid visibility → 400
   - Non-existent form ID → 404

3. **`02a-form-creation-org-membership-negative.http`** – Org Membership Error Path
   - Imports form-lifecycle setup + form creation, and `../organization-lifecycle/01-organization-creation.http`
   - Create form under org where user is not a member → 403

4. **`03-workflow-negative.http`** – Workflow Error Paths
   - Imports `../form-lifecycle/03-workflow-basic.http`
   - Non-existent form ID for workflow GET/POST → 404
   - Invalid node type / invalid body → 400

5. **`04-question-management-negative.http`** – Question CRUD Error Paths
   - Imports `../form-lifecycle/04-question-management.http`
   - Non-existent section ID → 404
   - Invalid question type, missing required fields → 400
   - Comprehensive negative cases for question operations

6. **`05-publishing-negative.http`** – Publishing Error Paths
   - Imports `../form-lifecycle/04-question-management.http`
   - Publish when workflow has condition node without rule/connections → 400
   - Partially connected workflow, non-existent form → 400 / 404

7. **`06-response-creation-negative.http`** – Response & Answer Error Paths
   - Imports `../form-lifecycle/06-response-creation.http`
   - Wrong questionType for question, invalid value type, value out of range → 400
   - Non-existent response/question IDs, invalid answer payloads → 404 / 400

8. **`07-highlight-negative.http`** – Highlight Error Paths
   - Imports `../form-lifecycle/06a-form-highlight.http`
   - Non-choice highlight question → 400
   - Non-existent form and missing highlight configuration → 404

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

4. **Prerequisite:** The positive form-lifecycle flow must be runnable (same env); negative tests import it for variables.

## Running Tests

### Run All Negative Tests (Sequential)

```bash
httpyac send scenarios/user-journey/negative-form-lifecycle/*.http
```

### Run Single Test File

```bash
httpyac send scenarios/user-journey/negative-form-lifecycle/01-setup-negative.http
```

## Dependencies

- **Positive form lifecycle** – `../form-lifecycle/` (01-setup through 06-response-creation) for auth and entity IDs
- **Organization lifecycle** – `../organization-lifecycle/01-organization-creation.http` for `02a` (org membership 403)
- **httpyac** – HTTP test runner
- **Node.js** – For assertions in `??` blocks
- Running Core System API instance

## Test Philosophy

- **Error-path only:** Each file runs the positive flow via `@import` to obtain variables, then executes only negative requests.
- **Assert status and body:** Tests assert HTTP status (404, 400, 403) and error payload fields (e.g. `title`, `status`).
- **Sequential execution:** Order matches the positive lifecycle; later negative files depend on variables from earlier positive/negative files.

## Troubleshooting

### Variable Not Found / Import Errors

- Run tests in order; negative files depend on positive form-lifecycle (and, for 02a, organization-lifecycle) imports.
- Ensure `../form-lifecycle/` and `../organization-lifecycle/` exist and their `.http` files are intact.

### Authentication Failures

- Same as form-lifecycle: verify `LOGIN_USER_ID` in `.env` and that internal login is enabled (e.g. debug mode).

### 404 / 400 Assertions Failing

- Confirm API error response shape (e.g. `title`, `status`) matches assertions; adjust test expectations if the API contract changed.

## Contributing

When adding new negative test files:

1. Follow the naming convention: `NN-description-negative.http` or `NNa-...-negative.http`.
2. Use `@import` from the corresponding positive flow to get variables.
3. Add clear comments for each error case (expected status and reason).
4. Use assertions (`??`) to validate status and error body.
5. Update this README with the new file and a short description.
