package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/index"
	"github.com/sourcegraph/zoekt/query"
	"github.com/sourcegraph/zoekt/search"
)

// IndexManager handles creating and managing code indexes
type IndexManager struct {
	indexDir string
}

// NewIndexManager creates a new index manager with the given base directory
func NewIndexManager(indexDir string) *IndexManager {
	return &IndexManager{indexDir: indexDir}
}

// GetIndexDir returns the base index directory
func (m *IndexManager) GetIndexDir() string {
	return m.indexDir
}

// getIndexPrefix returns a unique prefix for index files based on the source directory
func (m *IndexManager) getIndexPrefix(sourceDir string) string {
	hash := sha256.Sum256([]byte(sourceDir))
	hashStr := hex.EncodeToString(hash[:8])
	baseName := filepath.Base(sourceDir)
	// Sanitize base name to be safe for file names
	baseName = strings.ReplaceAll(baseName, " ", "_")
	return fmt.Sprintf("%s_%s", baseName, hashStr)
}

// IndexDirectory indexes the given source directory
func (m *IndexManager) IndexDirectory(sourceDir string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Ensure index directory exists
	if err := os.MkdirAll(m.indexDir, 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	// Delete any existing index files for this directory
	if err := m.deleteIndexFiles(absPath); err != nil {
		return fmt.Errorf("failed to clean up old index: %w", err)
	}

	// Create builder options - use flat structure with unique name prefix
	indexPrefix := m.getIndexPrefix(absPath)
	opts := index.Options{
		IndexDir: m.indexDir,
		RepositoryDescription: zoekt.Repository{
			Name:   indexPrefix,
			Source: absPath,
		},
	}
	opts.SetDefaults()

	// Create the builder
	builder, err := index.NewBuilder(opts)
	if err != nil {
		return fmt.Errorf("failed to create builder: %w", err)
	}

	// Walk the directory and add files
	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and common non-code directories
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || isSkippedDir(base) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files and non-text files
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		// Skip files that are likely binary
		if isBinaryFile(path) {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			// Skip files we can't read
			return nil
		}

		// Skip binary content
		if isBinaryContent(content) {
			return nil
		}

		// Get relative path from source directory
		relPath, err := filepath.Rel(absPath, path)
		if err != nil {
			return nil
		}

		// Add file to index
		doc := index.Document{
			Name:    relPath,
			Content: content,
		}

		return builder.Add(doc)
	})

	if err != nil {
		builder.Finish()
		return fmt.Errorf("failed to index files: %w", err)
	}

	// Finish building the index
	if err := builder.Finish(); err != nil {
		return fmt.Errorf("failed to finish index: %w", err)
	}

	// Save metadata about the indexed directory
	if err := m.saveIndexMetadata(absPath); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	return nil
}

// SearchOptions controls search behavior
type SearchOptions struct {
	MaxFiles        int  // Maximum number of files to return (default: 20)
	MaxLinesPerFile int  // Maximum matches per file (default: 3)
	MaxLineLength   int  // Truncate lines longer than this (default: 200)
	FilesOnly       bool // Only return file paths, no line content
}

// DefaultSearchOptions returns sensible defaults for context-efficient search
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		MaxFiles:        20,
		MaxLinesPerFile: 3,
		MaxLineLength:   200,
		FilesOnly:       false,
	}
}

// SearchResult holds the search output in a compact format
type SearchResult struct {
	TotalFiles   int      // Total number of files that matched
	TotalMatches int      // Total number of line matches
	Lines        []string // Compact output lines: "file:line: content" or just "file"
}

// Search performs a search across all indexes or a specific index
// Returns compact grep-like output to minimize context usage
func (m *IndexManager) Search(queryStr string, sourceDir string, opts SearchOptions) (*SearchResult, error) {
	// Apply defaults for zero values
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 20
	}
	if opts.MaxLinesPerFile <= 0 {
		opts.MaxLinesPerFile = 3
	}
	if opts.MaxLineLength <= 0 {
		opts.MaxLineLength = 200
	}

	// Always search in the base index directory (flat structure)
	searchDir := m.indexDir

	// If a specific directory is requested, add a repo filter to the query
	if sourceDir != "" {
		absPath, err := filepath.Abs(sourceDir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path: %w", err)
		}
		prefix := m.getIndexPrefix(absPath)
		// Add repo filter to query
		queryStr = fmt.Sprintf("repo:%s %s", prefix, queryStr)
	}

	// Load the searcher
	searcher, err := search.NewDirectorySearcher(searchDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	defer searcher.Close()

	// Parse the query
	q, err := query.Parse(queryStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// Set search options - request more than we need to get accurate totals
	zoektOpts := &zoekt.SearchOptions{
		MaxDocDisplayCount: opts.MaxFiles * 2, // Get extra for total count
	}

	// Perform the search
	result, err := searcher.Search(context.Background(), q, zoektOpts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Load metadata to map repo names to source directories
	metadata := m.loadAllMetadata()

	// Build compact output
	sr := &SearchResult{
		TotalFiles:   len(result.Files),
		TotalMatches: 0,
	}

	filesProcessed := 0
	for _, fileMatch := range result.Files {
		if filesProcessed >= opts.MaxFiles {
			break
		}
		filesProcessed++

		// Get source directory for full path
		baseDir := ""
		if meta, ok := metadata[fileMatch.Repository]; ok {
			baseDir = meta.SourceDir
		}

		fullPath := fileMatch.FileName
		if baseDir != "" {
			fullPath = filepath.Join(baseDir, fileMatch.FileName)
		}

		if opts.FilesOnly {
			sr.Lines = append(sr.Lines, fullPath)
			continue
		}

		// Collect matches from LineMatches
		linesAdded := 0
		for _, lineMatch := range fileMatch.LineMatches {
			sr.TotalMatches++
			if linesAdded >= opts.MaxLinesPerFile {
				continue
			}
			linesAdded++

			content := strings.TrimRight(string(lineMatch.Line), "\n\r")
			content = truncateLine(content, opts.MaxLineLength)

			sr.Lines = append(sr.Lines, fmt.Sprintf("%s:%d: %s",
				fullPath, lineMatch.LineNumber, content))
		}

		// Handle ChunkMatches if LineMatches is empty
		if len(fileMatch.LineMatches) == 0 {
			for _, chunk := range fileMatch.ChunkMatches {
				lines := strings.Split(string(chunk.Content), "\n")
				for i, line := range lines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					sr.TotalMatches++
					if linesAdded >= opts.MaxLinesPerFile {
						continue
					}
					linesAdded++

					content := truncateLine(strings.TrimRight(line, "\r"), opts.MaxLineLength)
					lineNum := int(chunk.ContentStart.LineNumber) + i

					sr.Lines = append(sr.Lines, fmt.Sprintf("%s:%d: %s",
						fullPath, lineNum, content))
				}
			}
		}

		// Add indicator if there are more matches in this file
		totalInFile := len(fileMatch.LineMatches)
		if totalInFile == 0 {
			for _, chunk := range fileMatch.ChunkMatches {
				totalInFile += strings.Count(string(chunk.Content), "\n") + 1
			}
		}
		if totalInFile > opts.MaxLinesPerFile {
			sr.Lines = append(sr.Lines, fmt.Sprintf("  ... and %d more matches in this file",
				totalInFile-opts.MaxLinesPerFile))
		}
	}

	// Add summary if results were truncated
	if sr.TotalFiles > opts.MaxFiles {
		sr.Lines = append(sr.Lines, fmt.Sprintf("\n[Showing %d of %d files. Use max_files to see more]",
			opts.MaxFiles, sr.TotalFiles))
	}

	return sr, nil
}

// truncateLine shortens a line to maxLen, adding ellipsis if truncated
func truncateLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// IndexInfo contains information about an index
type IndexInfo struct {
	Name      string `json:"name"`
	SourceDir string `json:"source_dir"`
}

// ListIndexes returns a list of all indexes
func (m *IndexManager) ListIndexes() ([]IndexInfo, error) {
	metadata := m.loadAllMetadata()

	var indexes []IndexInfo
	for name, meta := range metadata {
		indexes = append(indexes, IndexInfo{
			Name:      name,
			SourceDir: meta.SourceDir,
		})
	}

	return indexes, nil
}

// DeleteIndex removes an index for the given source directory
func (m *IndexManager) DeleteIndex(sourceDir string) error {
	absPath, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Delete the zoekt files
	if err := m.deleteIndexFiles(absPath); err != nil {
		return err
	}

	// Delete the metadata
	prefix := m.getIndexPrefix(absPath)
	metadata := m.loadAllMetadata()
	delete(metadata, prefix)

	return m.saveAllMetadata(metadata)
}

// indexMetadata stores information about an indexed directory
type indexMetadata struct {
	SourceDir string `json:"source_dir"`
}

func (m *IndexManager) getMetadataPath() string {
	return filepath.Join(m.indexDir, "metadata.json")
}

func (m *IndexManager) loadAllMetadata() map[string]*indexMetadata {
	metaPath := m.getMetadataPath()
	content, err := os.ReadFile(metaPath)
	if err != nil {
		return make(map[string]*indexMetadata)
	}

	var metadata map[string]*indexMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return make(map[string]*indexMetadata)
	}

	return metadata
}

func (m *IndexManager) saveAllMetadata(metadata map[string]*indexMetadata) error {
	content, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.getMetadataPath(), content, 0644)
}

func (m *IndexManager) saveIndexMetadata(sourceDir string) error {
	prefix := m.getIndexPrefix(sourceDir)
	metadata := m.loadAllMetadata()
	metadata[prefix] = &indexMetadata{SourceDir: sourceDir}
	return m.saveAllMetadata(metadata)
}

func (m *IndexManager) deleteIndexFiles(sourceDir string) error {
	prefix := m.getIndexPrefix(sourceDir)

	entries, err := os.ReadDir(m.indexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".zoekt") {
			if err := os.Remove(filepath.Join(m.indexDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// isSkippedDir returns true if the directory should be skipped
func isSkippedDir(name string) bool {
	skipDirs := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		"__pycache__":  true,
		"target":       true,
		"build":        true,
		"dist":         true,
		".git":         true,
		".svn":         true,
		".hg":          true,
		".idea":        true,
		".vscode":      true,
		"venv":         true,
		".venv":        true,
		"env":          true,
		".env":         true,
	}
	return skipDirs[name]
}

// isBinaryFile checks if a file is likely binary based on extension
func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".bin": true, ".obj": true, ".o": true, ".a": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true,
		".7z": true, ".rar": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".bmp": true, ".ico": true, ".webp": true, ".svg": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true,
		".xlsx": true, ".ppt": true, ".pptx": true,
		".class": true, ".jar": true, ".war": true,
		".pyc": true, ".pyo": true,
		".wasm": true,
	}
	return binaryExts[ext]
}

// isBinaryContent checks if content appears to be binary
func isBinaryContent(content []byte) bool {
	// Check first 8KB for null bytes
	checkLen := len(content)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}
