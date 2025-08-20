package annotation

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	basePath       = "internal/delivery/http"
	filePermission = 0644
	projectEnvVar  = "GITHUB_REPOSITORY"
	projectPrefix  = "omp-"
)

var (
	nameRegex  = regexp.MustCompile(`//\s*@name\s+\S+`)
	sourceDirs = []string{"request", "response"}
	basicTypes = map[string]bool{
		"string": true, "int": true, "int32": true, "int64": true,
		"uint": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true, "bool": true,
		"byte": true, "rune": true,
	}
)

// TypeInfo stores information about a type
type TypeInfo struct {
	Name        string
	IsGeneric   bool
	IsAlias     bool
	InnerType   string
	GenericBase string
}

// ProcessingResult tracks the results of processing files
type ProcessingResult struct {
	FilesProcessed      int
	AnnotationsAdded    int
	AnnotationsReplaced int
	Errors              []error
}

func (r *ProcessingResult) AddFile() {
	r.FilesProcessed++
}

func (r *ProcessingResult) AddAnnotation() {
	r.AnnotationsAdded++
}

func (r *ProcessingResult) ReplaceAnnotation() {
	r.AnnotationsReplaced++
}

func (r *ProcessingResult) AddError(err error) {
	r.Errors = append(r.Errors, err)
}

func (r *ProcessingResult) Summary() string {
	errorCount := len(r.Errors)
	return fmt.Sprintf("Processed %d files, added %d annotations, replaced %d annotations, %d errors",
		r.FilesProcessed, r.AnnotationsAdded, r.AnnotationsReplaced, errorCount)
}

func (r *ProcessingResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// safePrint handles fmt.Printf errors without panicking
func safePrint(format string, args ...interface{}) {
	if _, err := fmt.Printf(format, args...); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: print error: %v\n", err)
	}
}

func Run() error {
	projectPrefix := getProjectPrefix()

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	result := processSourceDirectories(root, projectPrefix)

	safePrint("%s\n", result.Summary())

	if result.HasErrors() {
		return fmt.Errorf("encountered %d errors during processing", len(result.Errors))
	}

	return nil
}

func processSourceDirectories(root, projectPrefix string) *ProcessingResult {
	result := &ProcessingResult{}

	for _, dir := range sourceDirs {
		fullPath := filepath.Join(root, basePath, dir)
		err := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err // Let filepath.Walk handle the error
			}

			if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}

			if err := processSourceFile(root, path, projectPrefix, dir, result); err != nil {
				// Collect errors but continue processing other files
				result.AddError(fmt.Errorf("processing %s: %w", path, err))
			}
			return nil
		})

		if err != nil {
			result.AddError(fmt.Errorf("walking directory %s: %w", fullPath, err))
		}
	}

	return result
}

func processSourceFile(root, path, projectPrefix, dir string, result *ProcessingResult) error {
	rel, err := filepath.Rel(filepath.Join(root, basePath), path)
	if err != nil {
		return fmt.Errorf("resolving relative path: %w", err)
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return nil // Skip files not in the expected structure
	}

	version := parts[1]
	prefix := fmt.Sprintf("%s.%s.", projectPrefix, version)

	result.AddFile()
	return processFile(path, prefix, dir, result)
}

func getProjectPrefix() string {
	repo := os.Getenv(projectEnvVar)
	if repo != "" {
		parts := strings.Split(repo, "/")
		if len(parts) == 2 {
			return getProjectName(parts[1])
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return getProjectName(filepath.Base(cwd))
	}
	return "project"
}

func getProjectName(input string) string {
	return strings.TrimPrefix(input, projectPrefix)
}

func extractTypeInfo(ts *ast.TypeSpec) TypeInfo {
	info := TypeInfo{Name: ts.Name.Name}

	if indexExpr, ok := ts.Type.(*ast.IndexExpr); ok {
		return handleIndexExpr(info, indexExpr, ts)
	}

	if ident, ok := ts.Type.(*ast.Ident); ok {
		return handleIdentType(info, ident)
	}

	if _, ok := ts.Type.(*ast.StructType); ok {
		return info
	}

	return TypeInfo{}
}

func handleIndexExpr(info TypeInfo, indexExpr *ast.IndexExpr, ts *ast.TypeSpec) TypeInfo {
	if ident, ok := indexExpr.X.(*ast.Ident); ok {
		if ts.Name.Name != ident.Name {
			info.IsAlias = true
		} else {
			info.IsGeneric = true
		}
		info.GenericBase = ident.Name
		if subType, ok := indexExpr.Index.(*ast.Ident); ok {
			info.InnerType = subType.Name
		}
	}
	return info
}

func handleIdentType(info TypeInfo, ident *ast.Ident) TypeInfo {
	if basicTypes[ident.Name] {
		return TypeInfo{} // Return empty for basic types
	}
	info.IsAlias = true
	return info
}

func generateAnnotationNameWithContext(info TypeInfo, variant string, searchResponseInnerTypes map[string]bool) (string, string) {
	if info.Name == "" {
		return "", ""
	}

	var mainName, itemName string

	if (info.IsGeneric || info.IsAlias) && info.GenericBase == "SearchResponse" {
		mainName = info.Name + "Res"
		if info.InnerType != "" {
			itemBase := strings.TrimSuffix(strings.TrimSuffix(info.InnerType, "Response"), "Request")
			if variant == "response" {
				itemName = itemBase + "ItemRes"
			} else {
				itemName = itemBase + "ItemReq"
			}
		}
	} else {
		base := strings.TrimSuffix(strings.TrimSuffix(info.Name, "Response"), "Request")
		if searchResponseInnerTypes[info.Name] {
			if variant == "response" {
				mainName = base + "ItemRes"
			} else {
				mainName = base + "ItemReq"
			}
		} else {
			if variant == "response" {
				mainName = base + "Res"
			} else {
				mainName = base + "Req"
			}
		}
	}

	return mainName, itemName
}

func processFile(filename, prefix, variant string, result *ProcessingResult) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing file: %w", err)
	}

	lines := bytes.Split(src, []byte("\n"))
	annotations := make(map[int]string)
	annotatedTypes := make(map[string]bool)
	ignoredTypes := make(map[string]bool)
	searchResponseInnerTypes := make(map[string]bool)

	// Multi-pass processing
	findIgnoredTypes(f, fset, lines, ignoredTypes)
	findSearchResponseInnerTypes(f, ignoredTypes, searchResponseInnerTypes)
	addAnnotations(f, fset, variant, ignoredTypes, searchResponseInnerTypes, annotations, annotatedTypes)

	// Apply annotations to lines
	annotationsApplied := applyAnnotations(lines, annotations, prefix, filename, result)

	if annotationsApplied > 0 {
		if err := writeFile(filename, lines); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
	}

	return nil
}

func findIgnoredTypes(f *ast.File, fset *token.FileSet, lines [][]byte, ignoredTypes map[string]bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !ts.Name.IsExported() {
			return true
		}

		// Check documentation comments
		if ts.Doc != nil {
			for _, comment := range ts.Doc.List {
				if strings.Contains(comment.Text, "@swagger:ignore") {
					ignoredTypes[ts.Name.Name] = true
					return true
				}
			}
		}

		// Check inline comments on the same line
		position := fset.Position(ts.Pos())
		if position.Line > 0 && position.Line <= len(lines) {
			lineContent := string(lines[position.Line-1])
			if strings.Contains(lineContent, "@swagger:ignore") {
				ignoredTypes[ts.Name.Name] = true
				return true
			}
		}

		// Check all comments in the file that might belong to this type
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.Contains(c.Text, "@swagger:ignore") {
					// Check if the comment is near the type
					commentPos := fset.Position(c.Pos())
					typePos := fset.Position(ts.Pos())
					if commentPos.Line >= typePos.Line-1 && commentPos.Line <= typePos.Line+1 {
						ignoredTypes[ts.Name.Name] = true
						return true
					}
				}
			}
		}

		return true
	})
}

func findSearchResponseInnerTypes(f *ast.File, ignoredTypes, searchResponseInnerTypes map[string]bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !ts.Name.IsExported() || ignoredTypes[ts.Name.Name] {
			return true
		}
		info := extractTypeInfo(ts)
		if (info.IsGeneric || info.IsAlias) && info.GenericBase == "SearchResponse" && info.InnerType != "" {
			searchResponseInnerTypes[info.InnerType] = true
		}
		return true
	})
}

func addAnnotations(f *ast.File, fset *token.FileSet, variant string, ignoredTypes, searchResponseInnerTypes map[string]bool, annotations map[int]string, annotatedTypes map[string]bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !ts.Name.IsExported() || ignoredTypes[ts.Name.Name] {
			return true
		}

		info := extractTypeInfo(ts)
		if info.Name == "" {
			return true
		}

		mainName, itemName := generateAnnotationNameWithContext(info, variant, searchResponseInnerTypes)

		if mainName != "" && !annotatedTypes[mainName] {
			endLine := fset.Position(ts.End()).Line - 1
			annotations[endLine] = mainName
			annotatedTypes[mainName] = true
		}

		if itemName != "" && !annotatedTypes[itemName] {
			if info.InnerType != "" {
				ast.Inspect(f, func(inner ast.Node) bool {
					if its, ok := inner.(*ast.TypeSpec); ok && its.Name.Name == info.InnerType && !ignoredTypes[its.Name.Name] {
						innerLine := fset.Position(its.End()).Line - 1
						annotations[innerLine] = itemName
						annotatedTypes[itemName] = true
						return false
					}
					return true
				})
			}
		}
		return true
	})
}

func applyAnnotations(lines [][]byte, annotations map[int]string, prefix, filename string, result *ProcessingResult) int {
	annotationsApplied := 0

	for i := range lines {
		if name, ok := annotations[i]; ok {
			expected := fmt.Sprintf("@name %s%s", prefix, name)
			lineStr := strings.TrimSpace(string(lines[i]))

			if strings.Contains(lineStr, expected) {
				continue // Annotation already exists
			}

			if loc := nameRegex.FindStringIndex(lineStr); loc != nil {
				// Replace the existing annotation
				lines[i] = append(lines[i][:loc[0]], []byte(fmt.Sprintf(" // %s", expected))...)
				safePrint("replaced annotation in %s: %s\n", filename, name)
				result.ReplaceAnnotation()
			} else {
				// Add new annotation
				annotation := fmt.Sprintf(" // %s", expected)
				lines[i] = append(lines[i], []byte(annotation)...)
				safePrint("added annotation to %s: %s\n", filename, name)
				result.AddAnnotation()
			}
			annotationsApplied++
		}
	}

	return annotationsApplied
}

func writeFile(filename string, lines [][]byte) error {
	var output bytes.Buffer
	if _, err := output.Write(bytes.Join(lines, []byte("\n"))); err != nil {
		return fmt.Errorf("writing to buffer: %w", err)
	}

	if err := os.WriteFile(filename, output.Bytes(), filePermission); err != nil {
		return fmt.Errorf("writing to file: %w", err)
	}

	return nil
}
