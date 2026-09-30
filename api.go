package martin

import intern "github.com/kyle-visner/martin/internal/martin"

// Public aliases for hosts and other AGPL integrators, such as AvianSuite
// serving Martin as hosted MCP tools. The domain engine stays in
// internal/martin; this file is the supported import surface.

type (
	Store              = intern.Store
	Context            = intern.Context
	Workspace          = intern.Workspace
	RemoteStoreOptions = intern.RemoteStoreOptions
	CRM                = intern.CRM
	Tool               = intern.Tool
	AppError           = intern.AppError
	ErrorCode          = intern.ErrorCode
)

const ActorOwner = "owner"

func OpenStore(dir string) (*Store, error) {
	return intern.OpenStore(dir)
}

func OpenRemoteStore(jaybaseURL, token string) (*Store, error) {
	return intern.OpenRemoteStore(jaybaseURL, token)
}

func OpenRemoteStoreWithOptions(jaybaseURL, token string, options RemoteStoreOptions) (*Store, error) {
	return intern.OpenRemoteStoreWithOptions(jaybaseURL, token, options)
}

// NewCRM binds a store to the acting identity. A host that serves Martin as
// MCP tools calls CRM.Invoke with the tool name and its JSON arguments, the
// same path the martin mcp command uses. Initialize the workspace first with
// Store.Initialize; init is not a tool.
func NewCRM(store *Store, ctx Context) *CRM {
	return intern.NewCRM(store, ctx)
}

// ToolCatalog lists every CRM operation as an MCP tool.
func ToolCatalog() []Tool {
	return intern.ToolCatalog()
}

// KnownTool reports whether name is in ToolCatalog.
func KnownTool(name string) bool {
	return intern.KnownTool(name)
}

// EncodeJSON renders a tool result the way the CLI and MCP server do.
func EncodeJSON(v any) ([]byte, error) {
	return intern.EncodeJSON(v)
}
