# Code Index MCP Server

An MCP (Model Context Protocol) server for fast local source code searching using [Zoekt](https://github.com/sourcegraph/zoekt).

## Features

- **Fast code search**: Uses Zoekt's trigram-based indexing for fast substring and regex matching
- **Per-directory indexes**: Each indexed directory gets its own index stored in the user profile
- **Rich query language**: Supports regex, file filters, language filters, and boolean operators
- **Cross-platform**: Works on macOS and Linux

## Installation

### From MCP Registry (Recommended)

This server is available on the [MCP Registry](https://registry.modelcontextprotocol.io/?q=trondhindenes%2Fcode-index). If you're using Claude Desktop or another MCP client that supports the registry, you can install it directly from there.

### Download Pre-built Binary

Download the appropriate `.mcpb` bundle for your platform from the [GitHub Releases](https://github.com/trondhindenes/code-index-mcp/releases) page:

- `code-index-mcp-darwin-arm64.mcpb` - macOS Apple Silicon
- `code-index-mcp-darwin-amd64.mcpb` - macOS Intel
- `code-index-mcp-linux-amd64.mcpb` - Linux x64
- `code-index-mcp-linux-arm64.mcpb` - Linux ARM64

The `.mcpb` file is a ZIP archive containing the binary and manifest. Extract it and place the `code-index-mcp` binary in your preferred location.

### Using Go

```bash
go install github.com/trondhindenes/code-index-mcp@latest
```

### Build from Source

```bash
git clone https://github.com/trondhindenes/code-index-mcp
cd code-index-mcp
go build -o code-index-mcp .
```

## Configuration

### Claude Desktop

Add to your Claude Desktop configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "code-index": {
      "command": "/path/to/code-index-mcp"
    }
  }
}
```

### Claude Code
Run the following:
```shell
claude mcp add --scope user --transport stdio code-index <path-to>/code-index-mcp
```

### Environment Variables

- `CODE_INDEX_DIR`: Override the default index storage location

Default index locations:
- macOS: `~/Library/Application Support/code-index/`
- Linux: `~/.local/share/code-index/` or `$XDG_DATA_HOME/code-index/`

## Available Tools

### `index_directory`

Index a source code directory for fast searching.

**Parameters:**
- `directory` (required): The path to the directory to index

**Example:**
```
Index the directory /Users/me/projects/myapp
```

### `search_code`

Search for code across indexed directories. Returns compact grep-like output to minimize context window usage.

**Parameters:**
- `query` (required): The search query using Zoekt syntax
- `directory` (optional): Limit search to a specific indexed directory
- `max_files` (optional): Maximum files to return (default: 20)
- `max_lines_per_file` (optional): Maximum matches per file (default: 3)
- `files_only` (optional): Only return file paths, no line content (default: false)

**Output Format:**
```
/path/to/file.go:42: matching line content here
/path/to/file.go:58: another matching line
  ... and 5 more matches in this file
```

**Query Syntax Examples:**
- `func main` - Simple text search
- `file:\.go$ func main` - Search only in .go files
- `lang:python class.*Model` - Search in Python files
- `-test func main` - Exclude files containing "test"
- `case:yes MyFunc` - Case-sensitive search

### `list_indexes`

List all indexed directories and their locations.

### `delete_index`

Delete the index for a specific directory.

**Parameters:**
- `directory` (required): The path to the directory whose index should be deleted

### `index_info`

Get information about the indexing configuration, including storage location.

## Skipped Directories

The following directories are automatically skipped during indexing:
- Hidden directories (starting with `.`)
- `node_modules`, `vendor`, `__pycache__`
- `target`, `build`, `dist`
- `venv`, `.venv`, `env`, `.env`

## Skipped Files

Binary files and common non-code files are automatically skipped, including:
- Executables (`.exe`, `.dll`, `.so`, `.dylib`)
- Archives (`.zip`, `.tar`, `.gz`)
- Images (`.png`, `.jpg`, `.gif`)
- Documents (`.pdf`, `.doc`, `.docx`)

## License

MIT
