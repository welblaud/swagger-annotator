# swagger-annotator

`swagger-annotator` is a lightweight CLI tool that automatically adds `@name` annotations to exported Go structs used in request/response folders, based on project and file path context. These annotations are used by Swagger generators (e.g. [swaggo/swag](https://github.com/swaggo/swag)) to assign schema names.

## ✅ What it does

* Detects exported structs in `internal/delivery/http/request/**` and `.../response/**`
* Automatically appends `// @name <prefix>.<version>.<TypeName>` to each exported struct
* Handles:

    * Aliases to generic types (like `SearchResponse[T]`)
    * Nested response types (e.g. item types inside a search response)
* Skips:

    * Primitive aliases (e.g. `type Foo string`)
    * Internal/helper structs marked to be ignored

## 🚫 Ignoring a struct

If you want to skip annotating a specific struct (e.g. internal helpers not exposed via Swagger), add the following comment at the end of the struct line:

```go
type VehiclePricingSearchNamer struct {
	ns schema.Namer
} // @swagger:ignore
```

This also prevents Swagger itself from including the type in generated documentation.

## 📦 Installation

Install the CLI globally using Go:

```bash
go install github.com/welblaud/swagger-annotator@latest
```

Then verify it’s in your path:

```bash
swagger-annotator -h
```

## 🛠️ Usage

### Annotate structs

To run the annotation process:

```bash
swagger-annotator -mode annotate
```

This will modify files in-place and update `@name` tags where needed.

### Validate annotations (CI mode)

You can use the `check` mode to fail CI if any annotation changes are needed:

```bash
swagger-annotator -mode check
```

This command:

* Runs the annotation logic in dry mode
* Fails with exit code `1` if changes are detected (uses `git diff --porcelain`)
* Use it in CI pipelines to ensure all annotations are committed

## 🧚‍ GitHub Action / Makefile

In your local workflow, you can define:

```makefile
swagger-annotator-install:
	go install github.com/welblaud/swagger-annotator@latest

swagger-annotator-annotate:
	swagger-annotator -mode annotate

swagger-annotator-check:
	swagger-annotator -mode check
```

In CI (e.g. GitHub Actions), use:

```yaml
- name: Install swagger-annotator
  run: go install github.com/welblaud/swagger-annotator@latest

- name: Check Swagger annotations
  run: |
    export PATH="$(go env GOPATH)/bin:$PATH"
    swagger-annotator -mode check
```

## 🧠 Project prefix detection

The annotation prefix is automatically derived from:

* The GitHub repository name (from `$GITHUB_REPOSITORY`, if set)
* Or your working directory name (as a fallback)

For example, in a repo named `omp-rental`, the following struct:

```go
type AdminRentalListResponse struct {
	// ...
}
```

Will receive the tag:

```go
} // @name rental.v1.AdminRentalListRes
```

## 🧼 Tip

Run this tool **before committing** your code. If annotations are missing or stale, CI pipelines using `-mode check` will fail until the annotations are up to date.

---
