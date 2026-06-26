# Form Lifecycle Test Scenarios

This directory contains comprehensive automated QA tests for the complete form lifecycle in the Core System API.

## Overview

These tests cover the full journey of a form from creation to archival, including:

- Form creation and management
- Question management (all types except OAuth)
- Basic workflow setup
- Form publishing
- Response submission and management
- Highlight configuration and statistics
- Form archival

## Test Files

Execute tests in the following order:

1. **`01-setup.http`** - Prerequisites & Authentication
   - Uses shared authentication from `../shared/common.http`
   - Creates test organization
   - Verifies initial state

2. **`02-form-creation.http`** - Form CRUD & Metadata
   - Creates new form
   - Tests form metadata updates
   - Tests form dressing (fonts, colors)
   - **`GET /forms/me`** — asserts each `UserForm` has required fields and **`responseIds` is always an array** (possibly empty before any responses exist)
   - Lists forms and sections

3. **`03-workflow-basic.http`** - Minimal Workflow Setup
   - Basic linear workflow (START → Section → END)
   - Section creation and workflow linking
   - Workflow validation

4. **`04-question-management.http`** - Question Types & CRUD
   - Tests all 12 non-OAuth question types
   - Question creation, update, deletion
   - Comprehensive CRUD operations

5. **`05-form-publishing.http`** - Publishing & Lifecycle
   - Form publishing
   - Status transitions (DRAFT → PUBLISHED)
   - Verification in listings

6. **`06-response-creation.http`** - Response Creation & Answer Submission
   - Create form response
   - **`GET /forms/me`** — after creating a response, asserts the form row’s **`responseIds`** contains that response id (`IN_PROGRESS`)
   - Submit answers for all question types
   - Test draft saving (auto-save) functionality
   - File upload and auto-answer creation
   - Final response submission
   - Verify response status transitions

7. **`06a-form-highlight.http`** - Form Highlight Lifecycle
   - Verify unconfigured highlight state
   - Set a choice-based highlight question
   - Verify per-choice response counts
   - Patch and clear highlight configuration

8. **`07-form-archiving.http`** - Form Archival & Cleanup
   - Archive form
   - Verify archived state in listings
   - Unarchive and re-publish form

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

3. Ensure the API server is running at the specified `BASE_URL`

## Running Tests

### Run All Tests (Sequential)

```bash
httpyac send scenarios/user-journey/form-lifecycle/*.http
```

### Run Single Test File

```bash
httpyac send scenarios/user-journey/form-lifecycle/01-setup.http
```

### Run with Specific Environment

```bash
httpyac send scenarios/user-journey/form-lifecycle/*.http --env dev
```

## Rich text / Tiptap support

### Supported Tiptap / ProseMirror types (backend-validated)

This section documents which ProseMirror node/mark types are **currently accepted by the backend validator** and observed to render via `descriptionHtml`.

Source of truth for this list is the scenario step `updateFormDescriptionNodeRenderers` in `02-form-creation.http`, validated end-to-end by running:

```bash
httpyac send -a --bail 06-response-creation.http
```

#### Nodes

- **`doc`**: ProseMirror document root (`type: "doc"`).
- **`heading`**: `attrs.level` supported (observed: 1, 2, 3, 4, 5, 6).
- **`paragraph`**
- **`text`**
- **`hard_break`**
- **`horizontal_rule`**
- **`blockquote`**: supports multiple paragraphs and nested lists.
- **`code_block`**: supports multi-line text.
- **`bullet_list`**
- **`ordered_list`**: supports `attrs.order`.
- **`list_item`**: supports nesting lists within list items.
- **`variable`**: supports `attrs.name` (rendered as `{{NAME}}` in `descriptionHtml`).

#### Marks

- **`bold`** (renders as `<strong>…</strong>`)
- **`italic`** (renders as `<em>…</em>`)
- **`underline`** (renders as `<u>…</u>` or equivalent styling)
- **`code`** (inline code mark)
- **`link`**: observed-supported href forms:
  - **HTTP(S)** absolute URLs (e.g. `https://example.com`)
  - **Relative-path style** accepted when represented as `https://example.com/relative/path` in the test
  - **`mailto:`** (e.g. `mailto:test@example.com`)
  - **Pure hash** (e.g. `#hash`)

#### Rendering / sanitization notes

- **`descriptionHtml`** is expected to be safe for frontend rendering; the smoke test includes a string `"<script>alert(1)</script>"` and asserts:
  - `<script` is **not** present in `descriptionHtml`
  - `alert(1)` text **is** still visible
- **Emoji**: unicode emoji characters embedded as plain text are preserved (e.g. `😀`, `👨‍👩‍👧‍👦`).

## Known Limitations

### OAuth Question Type Not Tested

The `OAUTH_CONNECT` question type and OAuth-related answer endpoints are **not included** in these tests because:

- OAuth flows require browser interaction and external provider setup
- Cannot be easily automated in API testing without complex mocking
- Requires valid OAuth credentials and callback handling

**Endpoints Excluded:**

- `GET /oauth/questions/{provider}` - OAuth account connection

**Question Types Excluded:**

- `OAUTH_CONNECT`

All other 12 question types are fully tested:

- SHORT_TEXT
- LONG_TEXT
- SINGLE_CHOICE
- MULTIPLE_CHOICE
- DROPDOWN
- DETAILED_MULTIPLE_CHOICE
- DATE
- UPLOAD_FILE
- LINEAR_SCALE
- RATING
- RANKING
- HYPERLINK

### Google Sheet Integration

Google Sheet verification tests (in `05-optional-google-sheet.http`) may fail without proper setup:

- Requires valid Google service account permissions
- Service account email must be added to target Google Sheet
- Tests are marked as optional and won't block the test suite

## Test Philosophy

- **Non-blocking Optional Tests:** Google Sheet and file upload tests log warnings on failure
- **Minimal Workflow:** Basic workflow to avoid blocking subsequent tests; complex workflows tested separately in `form-workflow` scenarios
- **Comprehensive Coverage:** All API endpoints tested except OAuth flows
- **Sequential Execution:** Tests build on previous test results using httpyac variable references

## Dependencies

- **httpyac** - HTTP test runner
- **Node.js** - For scripting and assertions
- Running Core System API instance

## Troubleshooting

### Authentication Failures

- Verify `LOGIN_USER_ID` in `.env` is a valid user UUID
- Check that internal login endpoint is enabled (debug mode)

### 404 Errors

- Ensure API server is running at `BASE_URL`
- Verify API version matches (`/api/...` prefix)

### Variable Not Found

- Run tests in sequential order
- Previous test files export variables needed by later tests

## Contributing

When adding new test files:

1. Follow the naming convention: `NN-description.http`
2. Add clear comments explaining each test section
3. Use assertions (`??`) to validate responses
4. Export variables using `@variableName = {{response.body.field}}`
5. Update this README with new test descriptions
