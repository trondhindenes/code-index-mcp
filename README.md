# Code Index MCP Server

An MCP (Model Context Protocol) server for fast local source code searching using [Zoekt](https://github.com/sourcegraph/zoekt).

## Features

- **Fast code search**: Uses Zoekt's trigram-based indexing for fast substring and regex matching
- **Per-directory indexes**: Each indexed directory gets its own index stored in the user profile
- **Rich query language**: Supports regex, file filters, language filters, and boolean operators
- **Cross-platform**: Works on macOS and Linux

## Installation

### From MCP Registry (Recommended)

This server is available on the [MCP Registry](https://registry.modelcontextprotocol.io/?q=trondhindenes%2Fcode-index). You can install it using the MCPB CLI:

```bash
# Install mcpb CLI if you haven't already
npm install -g @anthropic-ai/mcpb

# Install the server
mcpb install io.github.trondhindenes/code-index-mcp
```

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
