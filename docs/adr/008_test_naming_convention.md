# ADR-008: Go Test Naming Convention

- **Status:** Proposed
- **Date:** 2025-04-19

## Context

A clear, descriptive, and consistent test naming convention is needed across the CowGnition Go codebase. This improves test suite readability, maintainability, and makes it easier to understand the purpose of each test case at a glance. Current test names primarily indicate the component and success/failure, but could be more descriptive of the specific behavior being tested. We reviewed existing project conventions and external strategies, including Given-When-Then (BDD) and the "Behaviours" strategy discussed by Diogo Mateus. The goal is to adopt a convention that clearly links test cases to specific component behaviors under defined conditions.

## Decision

We will adopt the **"Behaviours" strategy** for naming Go unit and integration tests within the CowGnition project.

**Format:**

`Test[Component]_[ExpectedBehaviour]_When_[StateUnderTest]`

-   **`[Component]` (Optional but Recommended):** The specific component, type, or unit being tested (e.g., `Validator`, `RTMClient`, `MCPHandler`, `ServerRunner`). Prefixing with the component helps organize tests, especially in packages with multiple distinct units under test.
-   **`[ExpectedBehaviour]`:** Describes what the component *should do* or the *outcome* expected from the test. Use present tense verbs (e.g., `SucceedsInitialization`, `FailsValidation`, `ReturnsError`, `ReturnsEmptyList`, `Panics`).
-   **`_When_`:** A separator word for readability, clearly delineating the expected behavior from the condition causing it.
-   **`[StateUnderTest]`:** Describes the specific condition, input state, or scenario that leads to the `ExpectedBehaviour` (e.g., `UsingValidSchemaFile`, `SchemaFileNotFound`, `MessageTypeIsIncorrect`, `GivenNilInput`, `AuthTokenIsInvalid`, `FilterIsEmpty`).

**Examples:**

```go
// internal/schema/validator_test.go
func TestValidator_SucceedsInitialization_When_UsingValidSchemaFile(t *testing.T) { ... }
func TestValidator_FailsInitialization_When_SchemaFileNotFound(t *testing.T) { ... }
func TestValidator_FailsInitialization_When_SchemaFileIsInvalidJSON(t *testing.T) { ... }
func TestValidator_FailsValidation_When_MessageTypeIsIncorrect(t *testing.T) { ... }
func TestValidator_FailsValidation_When_RequiredFieldMissing(t *testing.T) { ... }

// internal/rtm/client_test.go
func TestRTMClient_SucceedsEcho_When_CredentialsAreValid(t *testing.T) { ... }
func TestRTMClient_ReturnsAuthError_When_TokenIsInvalid(t *testing.T) { ... }
func TestRTMClient_ReturnsTasks_When_AuthenticatedWithValidFilter(t *testing.T) { ... }

// internal/mcp/mcp_server_test.go
func TestMCPServer_RejectsRequest_When_NotInitialized(t *testing.T) { ... }
func TestMCPServer_ReturnsMethodNotFound_When_MethodIsUnknown(t *testing.T) { ... }
Important Note on Assertions:

This naming convention defines the scenario being tested. The test body MUST still contain detailed assertions to verify the specifics of the success or failure. This includes:

Using require.Error / assert.NoError appropriately.
Using errors.As to check for specific custom error types (e.g., *schema.ValidationError, *mcperrors.RTMError).
Using errors.Is to check for specific sentinel errors.
Asserting specific error codes or fields within custom errors (e.g., assert.Equal(t, schema.ErrSchemaNotFound, validationErr.Code)).
Asserting that error messages or data fields contain specific, relevant details (e.g., assert.Contains(t, validationErr.Message, "schema file not found"), assert.Equal(t, "/params/filter", validationErr.InstancePath)).
Asserting expected values in successful results.
The name sets the stage; the assertions prove the details.

Consequences
Positive
Improved Readability: Test names clearly state the expected outcome and the conditions under which it occurs.
Consistency: Provides a standard structure for all tests across the project.
Maintainability: Easier to understand the purpose of a test when reviewing or debugging failures.
Requirement Mapping: Test names more closely reflect the behavioral requirements being verified.
Negative
Refactoring Effort: Existing test names will need to be refactored to conform to the new convention.
Team Discipline: Requires consistent application by all developers.
Potential Verbosity: Names can become slightly longer than simpler conventions, though generally shorter than full Given-When-Then.
References
Martin Fowler - GivenWhenThen
Diogo Mateus - Naming tests in Golang
