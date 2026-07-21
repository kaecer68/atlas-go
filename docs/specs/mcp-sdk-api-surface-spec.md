# MCP SDK API Surface — Sampling / Roots / Elicitation

> **Scope**: Spike output for Phase 4 T2.1. Documents the `go-mcp-sdk` v1.6.1 API surface for the three MCP 2026-07-28 protocol primitives so that T2.2–T2.4 production handlers can be designed without re-grepping the SDK source.
> **Status**: Spike (temporary; this doc may be merged into `agent-mcp-phase4.md` §6.2 when Direction B is approved).
> **Related**: [`agent-mcp-phase4.md`](./agent-mcp-phase4-spec.md), [`agent-mcp-server.md`](./agent-mcp-server-spec.md), internal/apigateway/CONSTITUTION-spec.md.)

---

## 1. SDK Version

```bash
go list -m github.com/modelcontextprotocol/go-sdk
# github.com/modelcontextprotocol/go-sdk v1.6.1
```

- **Current version**: `v1.6.1`
- **Bump needed**: `NO` — the spec (`agent-mcp-phase4.md` §4.2) already noted that v1.6.1 natively exposes the three primitives.
- **Module path**: `github.com/modelcontextprotocol/go-sdk/mcp`

---

## 2. Shared Types

```go
// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go
type Implementation struct {
    Name       string `json:"name"`
    Title      string `json:"title,omitempty"`
    Version    string `json:"version"`
    WebsiteURL string `json:"websiteUrl,omitempty"`
    Icons      []Icon `json:"icons,omitempty"`
}

type ServerOptions struct {
    // ... other fields ...
    RootsListChangedHandler func(context.Context, *RootsListChangedRequest)
    // ...
}

type ClientOptions struct {
    // ... other fields ...
    CreateMessageHandler          func(context.Context, *CreateMessageRequest) (*CreateMessageResult, error)
    CreateMessageWithToolsHandler func(context.Context, *CreateMessageWithToolsRequest) (*CreateMessageWithToolsResult, error)
    ElicitationHandler            func(context.Context, *ElicitRequest) (*ElicitResult, error)
    Capabilities                  *ClientCapabilities
    // ...
}

type ClientCapabilities struct {
    RootsV2     *RootCapabilities        `json:"-"`
    Sampling    *SamplingCapabilities    `json:"sampling,omitempty"`
    Elicitation *ElicitationCapabilities `json:"elicitation,omitempty"`
    // ...
}

type RootCapabilities struct {
    ListChanged bool `json:"listChanged,omitempty"`
}

type SamplingCapabilities struct {
    Context *SamplingContextCapabilities `json:"context,omitempty"`
    Tools   *SamplingToolsCapabilities   `json:"tools,omitempty"`
}

type ElicitationCapabilities struct {
    Form *FormElicitationCapabilities `json:"form,omitempty"`
    URL  *URLElicitationCapabilities  `json:"url,omitempty"`
}
```

---

## 3. Sampling

### 3.1 Server-side entry point

```go
// github.com/modelcontextprotocol/go-sdk/mcp/server.go:1186
func (ss *ServerSession) CreateMessage(
    ctx context.Context,
    params *CreateMessageParams,
) (*CreateMessageResult, error)

// github.com/modelcontextprotocol/go-sdk/mcp/server.go:1223
func (ss *ServerSession) CreateMessageWithTools(
    ctx context.Context,
    params *CreateMessageWithToolsParams,
) (*CreateMessageWithToolsResult, error)
```

### 3.2 Request / result types

```go
// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:395
type CreateMessageParams struct {
    Meta             `json:"_meta,omitempty"`
    IncludeContext   string             `json:"includeContext,omitempty"`
    MaxTokens        int64              `json:"maxTokens"`
    Messages         []*SamplingMessage `json:"messages"`
    Metadata         any                `json:"metadata,omitempty"`
    ModelPreferences *ModelPreferences  `json:"modelPreferences,omitempty"`
    StopSequences    []string           `json:"stopSequences,omitempty"`
    SystemPrompt     string             `json:"systemPrompt,omitempty"`
    Temperature      float64            `json:"temperature,omitempty"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:433
type CreateMessageWithToolsParams struct {
    Meta             `json:"_meta,omitempty"`
    IncludeContext   string               `json:"includeContext,omitempty"`
    MaxTokens        int64                `json:"maxTokens"`
    Messages         []*SamplingMessageV2 `json:"messages"`
    Metadata         any                  `json:"metadata,omitempty"`
    ModelPreferences *ModelPreferences    `json:"modelPreferences,omitempty"`
    StopSequences    []string             `json:"stopSequences,omitempty"`
    SystemPrompt     string               `json:"systemPrompt,omitempty"`
    Temperature      float64              `json:"temperature,omitempty"`
    Tools            []*Tool              `json:"tools,omitempty"`
    ToolChoice       *ToolChoice          `json:"toolChoice,omitempty"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:532
type CreateMessageResult struct {
    Meta       `json:"_meta,omitempty"`
    Content    Content `json:"content"`
    Model      string  `json:"model"`
    Role       Role    `json:"role"`
    StopReason string  `json:"stopReason,omitempty"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:577
type CreateMessageWithToolsResult struct {
    Meta       `json:"_meta,omitempty"`
    Content    []Content `json:"content"`
    Model      string    `json:"model"`
    Role       Role      `json:"role"`
    StopReason string    `json:"stopReason,omitempty"`
}
```

### 3.3 Client-side handler

```go
// github.com/modelcontextprotocol/go-sdk/mcp/requests.go:26-27
type CreateMessageRequest          = ClientRequest[*CreateMessageParams]
type CreateMessageWithToolsRequest = ClientRequest[*CreateMessageWithToolsParams]

// github.com/modelcontextprotocol/go-sdk/mcp/client.go:81
type ClientOptions struct {
    CreateMessageHandler func(context.Context, *CreateMessageRequest) (*CreateMessageResult, error)
    // ...
}
```

### 3.4 Required imports (server)

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"
```

### 3.5 Caveats

- `CreateMessage` down-converts the client response from `[]Content` to a single `Content`; if the client returns multiple content blocks, it returns an error and you must use `CreateMessageWithTools`.
- Setting **both** `CreateMessageHandler` and `CreateMessageWithToolsHandler` in `ClientOptions` panics at client construction.
- The client must advertise `sampling` capability; the SDK infers it automatically when either handler is set.
- Per `agent-mcp-phase4.md` §8, sampling should be **opt-in / default off** because every call adds LLM latency and cost.

---

## 4. Roots

### 4.1 Server-side entry point

```go
// github.com/modelcontextprotocol/go-sdk/mcp/server.go:1174
func (ss *ServerSession) ListRoots(
    ctx context.Context,
    params *ListRootsParams,
) (*ListRootsResult, error)
```

### 4.2 Request / result types

```go
// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:829
type ListRootsParams struct {
    Meta `json:"_meta,omitempty"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:842
type ListRootsResult struct {
    Meta  `json:"_meta,omitempty"`
    Roots []*Root `json:"roots"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:1187
type Root struct {
    Meta `json:"_meta,omitempty"`
    Name string `json:"name,omitempty"`
    URI  string `json:"uri"` // must start with file:// (for now)
}
```

### 4.3 Change-notification handler

```go
// github.com/modelcontextprotocol/go-sdk/mcp/server.go:73
type ServerOptions struct {
    RootsListChangedHandler func(context.Context, *RootsListChangedRequest)
    // ...
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:1201
type RootsListChangedParams struct {
    Meta `json:"_meta,omitempty"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/requests.go:20
type RootsListChangedRequest = ServerRequest[*RootsListChangedParams]
```

### 4.4 Client-side helpers

```go
// Client.AddRoots(roots ...*Root)
// Client.RemoveRoots(uris ...string)
```

### 4.5 Required imports (server)

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"
```

### 4.6 Caveats

- `Root.URI` is currently restricted to `file://` scheme.
- Clients advertise roots by default with `listChanged: true`; to disable roots entirely, pass `Capabilities: &mcp.ClientCapabilities{}`.
- Because roots expose filesystem paths, any atlas-mcp usage must respect `internal/apigateway/CONSTITUTION.md`:
  - Do not perform arbitrary file reads outside an audited boundary.
  - Default to **read-only** access (per `agent-mcp-phase4.md` §8 and §9).
  - Write access, if ever added, requires explicit opt-in + audit logging + apigateway architecture review (T2.3 scope, not T2.1).

---

## 5. Elicitation

### 5.1 Server-side entry point

```go
// github.com/modelcontextprotocol/go-sdk/mcp/server.go:1239
func (ss *ServerSession) Elicit(
    ctx context.Context,
    params *ElicitParams,
) (*ElicitResult, error)
```

### 5.2 Request / result types

```go
// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:1437
type ElicitParams struct {
    Meta            `json:"_meta,omitempty"`
    Mode            string `json:"mode"`            // "form" | "url"
    Message         string `json:"message"`
    RequestedSchema any    `json:"requestedSchema,omitempty"`
    URL             string `json:"url,omitempty"`
    ElicitationID   string `json:"elicitationId,omitempty"`
}

// github.com/modelcontextprotocol/go-sdk/mcp/protocol.go:1477
type ElicitResult struct {
    Meta    `json:"_meta,omitempty"`
    Action  string         `json:"action"`  // "accept" | "decline" | "cancel"
    Content map[string]any `json:"content,omitempty"`
}
```

### 5.3 Client-side handler

```go
// github.com/modelcontextprotocol/go-sdk/mcp/requests.go:28
type ElicitRequest = ClientRequest[*ElicitParams]

// github.com/modelcontextprotocol/go-sdk/mcp/client.go:102
type ClientOptions struct {
    ElicitationHandler func(context.Context, *ElicitRequest) (*ElicitResult, error)
    // ...
}
```

### 5.4 Required imports (server)

```go
import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
    // If constructing RequestedSchema values in Go:
    "github.com/google/jsonschema-go/jsonschema"
)
```

### 5.5 Caveats

- `Mode` is inferred if empty:
  - `"url"` if `URL != ""` or `ElicitationID != ""`
  - otherwise `"form"`
- The SDK validates `RequestedSchema` when it is non-nil:
  - Root schema type must be `"object"`.
  - Only **top-level primitive properties** are allowed (no nested objects).
  - Allowed property types: `string`, `number`, `integer`, `boolean`, `array`.
  - Allowed string formats: `email`, `uri`, `date`, `date-time`.
  - Enums are supported but only for string properties.
- If `Action != "accept"`, the SDK skips schema validation.
- The server must check that the client declared `elicitation` capability before calling `Elicit`; otherwise the SDK returns `client does not support elicitation`.

---

## 6. Existing atlas-mcp Usage

The `cmd/atlas-mcp/` tree already imports `github.com/modelcontextprotocol/go-sdk/mcp`:

```bash
grep -rl "modelcontextprotocol/go-sdk/mcp" cmd/atlas-mcp/
# cmd/atlas-mcp/server/*.go
# cmd/atlas-mcp/internal/descgen/extract_test.go
```

Current usage is limited to tool/resource/prompt registration and the base `mcp.Server`. No production code currently calls `CreateMessage`, `ListRoots`, or `Elicit`.

---

## 7. Open Questions for User Checkpoint #1

These map to the questions in `agent-mcp-phase4.md` §9:

1. **Sampling default**: Keep sampling **default off** and require an explicit flag; optionally log a one-line warning at startup indicating how to enable it.
2. **Roots access**: Keep roots **read-only** by default. Any write capability should be opt-in and deferred to Phase 5 / a separate apigateway review.
3. **Elicitation schema**: Accept the SDK's built-in validation (flat object, primitive properties only, max depth 1). Document this limit in the production handler so callers are not surprised.

---

## 8. Spike Artifacts

- `cmd/atlas-mcp-server-sdk-spike/spike.go` — temporary helper functions exercising the three server-side APIs.
- `cmd/atlas-mcp-server-sdk-spike/spike_test.go` — three tests verifying compile + basic in-memory call for sampling, roots, and elicitation.
- Verification command: `go test -race ./cmd/atlas-mcp-server-sdk-spike/...`

---

## 9. References

- `go-mcp-sdk` v1.6.1 source: `$(go env GOPATH)/pkg/mod/github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp/`
- MCP 2026-07-28 spec: <https://modelcontextprotocol.io/specification/2026-07-28>
- `docs/specs/agent-mcp-phase4-spec.md` §4.2, §6.2, §7, §8, §9
- `internal/apigateway/CONSTITUTION.md` (filesystem / data-source governance)
