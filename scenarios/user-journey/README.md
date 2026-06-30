# User Journey `journey.yaml` Guide

This directory contains API end-to-end scenarios executed by `httpyac-action`. Each scenario folder should contain:

- one `journey.yaml`
- one or more `.http` files
- an optional scenario-specific `README.md`

`journey.yaml` is the entry configuration used by the test runner to decide:

- which journey is shown in the report
- which `.http` file to execute
- which `# @name` testcase inside that file should be used as the execution target

## Current Format

Use the following structure:

```yaml
name: "Form Lifecycle"
description: "Positive end-to-end coverage for the form lifecycle, including highlight management and archive recovery."
cases:
  - name: "Response Submission And Cancellation"
    description: "Verify response submission, response listing, cancellation of a submitted response, and the final draft state after cancellation."
    path: "06-response-creation.http"
    test: "getResponseAfterCancel"

  - name: "Highlight Lifecycle"
    description: "Verify highlight configuration, statistics, title updates, and cleanup on a published form."
    path: "06a-form-highlight.http"
    test: "getHighlightAfterClear"

  - name: "Archive And Restore"
    description: "Verify archiving, listing visibility changes, unarchive behavior, and successful re-publication."
    path: "07-form-archiving.http"
    test: "verifyFormRepublished"
```

## Field Definitions

### Top-level fields

- `name`: Human-readable journey name shown in reports.
- `description`: Short summary of what the whole journey covers.
- `cases`: A required array of executable test cases.

### Case fields

- `name`: Human-readable case title shown in reports.
- `description`: Short summary of what this case verifies.
- `path`: Relative path to the target `.http` file inside the same scenario folder.
- `test`: The exact `# @name` value in that `.http` file.

## How Execution Works

When `httpyac-action` runs a case, it executes:

1. the `.http` file specified by `path`
2. the testcase specified by `test`

In practice, the selected testcase usually depends on previous requests in the same file through `# @ref`, and the file itself may depend on earlier files through `# @import`.

Because of that, a single case often covers much more than one request.

However, `# @import` alone does not mean every testcase in the imported file will be executed. Actual coverage is determined by the selected testcase and the `# @ref` chain required to reach it.

For example:

- `07-form-archiving.http` imports `06-response-creation.http`
- `verifyFormRepublished` depends on all prior requests in `07-form-archiving.http`

So running that one case already covers:

- form publishing prerequisites
- response creation prerequisites
- archive flow
- unarchive flow
- re-publish verification

But it does not automatically cover later branches in `06-response-creation.http`. For example, `cancelSubmittedResponse` is only exercised when the selected target reaches `getResponseAfterCancel`.

## Minimal Template

```yaml
name: "Journey Name"
description: "Short summary of the journey."
cases:
  - name: "Case Name"
    description: "Short summary of what this case verifies."
    path: "NN-scenario.http"
    test: "finalAssertionName"
```
