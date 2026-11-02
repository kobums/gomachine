# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based code generation tool called "gomachine" that generates models, controllers, and data access layers from database schemas. It supports both Go and Dart/Flutter code generation using Jet templates.

## Key Commands

### Build and Development
- `make install` - Builds and installs the tool to `~/bin/buildtool-model`
- `make run` - Runs the model generator directly
- `make model` - Builds the model generator binary
- `make test` - Runs tests using `go test -v ./...`
- `make clean` - Removes local `bin/` and the files `make install` placed in `~/bin` (`buildtool-model*`, `buildtool/`)

### Running the Tool
The main executable takes these arguments:
```bash
buildtool-model <target_path> [language] [package_name]
```
- `target_path`: Root directory of the target project
- `language`: overrides model.json's language selection. One or comma-separated: "go", "kotlin"/"spring", "dart"/"flutter", "react", "vue", or "all"
- `package_name`: Optional package name (auto-detected from go.mod if not provided)

## Architecture

### Core Components

1. **Configuration System** (`config/`)
   - `config.json`: Database connection and generation settings
   - `config.go`: Configuration structures and JSON parsing

2. **Template Engine** (`views/`)
   - Uses CloudyKit Jet templating engine
   - `views/go/`: Go templates (model.jet, rest.jet, const.jet, domain_router.jet)
   - `views/dart/`: Dart/Flutter templates (model.jet, params.jet, provider.jet, repository.jet)
   - `views/kotlin/`: Kotlin/Spring Boot templates (entity.jet, repository.jet, service.jet, controller.jet)
   - `views/react/`: React TypeScript templates (contract.jet, model.jet, type.jet)
   - `views/vue/`: Vue 3 + TypeScript templates (contract.jet, type.jet, model.jet, composable.jet; contract/model/type are kept identical to the react ones). Template data comes from `react.BuildTemplateData`

3. **Code Generation** (`main.go`, `go/`, `dart/`, `kotlin/`)
   - Connects to MySQL database via `information_schema`
   - Generates models, controllers, and data access code
   - Supports custom method generation via configuration
   - Multi-language support: Go, Dart/Flutter, Kotlin/Spring Boot

### Database Integration

- Uses MySQL with connection pooling
- Queries `information_schema.tables` and `information_schema.columns` (columns in `ordinal_position` order)
- Generates code based on database schema and naming conventions
- Table naming: `prefix_tablename` format where prefix becomes model prefix

### Generated Code Structure

**For Go:**
- Models: `./models/{tablename}.go`
- Controllers: `./controllers/rest/{tablename}.go`
- Constants: `./models/{tablename}/{tablename}.go`
- Routes: `./router/routers/{tablename}.go`

**For Dart/Flutter:**
- Generates to `../gym/gym/lib/{type}/{tablename}_{type}.dart`
- Types: model, params, provider, repository

**For Kotlin/Spring Boot:**
- Written under `kotlinModelFilePath` (must point at the package directory, e.g. `.../src/main/kotlin/com/example/app`):
  `entity/{ModelName}.kt`, `repository/{ModelName}Repository.kt`, `service/{ModelName}Service.kt`, `controller/{ModelName}Controller.kt`, `enums/{model}/Enums.kt`
- Package is `kotlinPackage` from model.json (default `com.gowoobro.gymspring`)
- Creates complete REST API with JPA entities, repositories, services, and controllers

### Configuration Schema

The `config.json` supports:
- Database connection settings
- Language selection: `languages` map of on/off switches (`{"go": true, "react": true, "spring": false}`, `"all": true` enables every generator). Falls back to the legacy `language` string, then to `go`. Resolution lives in `config.EnabledLanguages()` / `config.ResolveLanguages()`
- Per-language output paths: `goModelFilePath`, `kotlinModelFilePath`, `dartModelFilePath`, `reactModelFilePath`, `vueModelFilePath`. Non-Go languages without a path are skipped with a warning
- Custom method generation via `table` array with GPA (Generation Parameter Array) objects
- Join configurations for related tables
- Session parameter mappings

### Type Mapping

`util.GetType` maps MySQL `data_type` per language. Unknown types log a warning and fall back to `string`/`String`.

| MySQL | Go | Kotlin | Dart |
|---|---|---|---|
| `int`, `integer`, `smallint`, `mediumint`, `year` | `int` | `Int` | `int` |
| `bigint` | `int64` | `Long` | `int` |
| `tinyint` | `bool` | `Int` | `bool` |
| `float`, `double`, `real` | `float64` | `Double` | `double` |
| `decimal`, `numeric` | `float64` | `BigDecimal` | `double` |
| `char`, `varchar`, `*text`, `enum`, `set`, `json` | `string` | `String` | `String` |
| `datetime`, `timestamp` | `string` | `LocalDateTime` | `String` |
| `date` | `string` | `LocalDate` | `String` |
| `time` | `string` | `String` | `String` |
| `binary`, `varbinary`, `*blob` | `[]byte` | `ByteArray` | `String` |
| `bit` | `[]byte` (driver returns raw bytes) | `Boolean` | `bool` |

Columns listed in `table[].map` become enum types (Go: `{table}.{Name}` in `models/{table}`, Kotlin/Dart: `{Name}`).

### Naming

- Tables: `_tb` / `_vw` suffix is stripped (`user_role_tb` → model `UserRole`, files/packages/URLs `userrole`). `_vw` tables are read-only (no insert/update/delete code). If two tables produce the same name (e.g. `user_tb` and `user_vw`) the `_tb` keeps it and the other is renamed (`UserView`).
- Columns: the prefix before the first `_` is dropped (`p_created_at` → key `created_at`, Go field `CreatedAt`, JSON key `createdAt`). Single-underscore columns keep the old names (`u_createddate` → `Createddate` / `createddate`).
- Columns that are Go/Kotlin keywords get safe identifiers (Go parameter `type_`, Kotlin `` `class` ``).
- JSON keys / filter params / search paths come from `util.JSONKey`, `util.SearchSegment`, `util.RangeParams` and must be used by every generator (Go json tags, Kotlin properties, TS fields, Dart json keys). `json_keys_test.go` checks they agree.
- Custom methods (`table[].method`) are parsed by `util.ParseFuncs` (also used by the TS search methods); methods that don't match columns are skipped with a warning.

### HTTP contract

React/Vue/Dart must work against both the Go and the Spring backend. README.md "HTTP 계약" documents the routes, response shapes and error handling. Rules: on the Go side only add routes/fields (existing Go clients depend on the old ones); register static routes before `/:id`; TS response normalization lives in the generated `src/models/contract.ts`.

## Dependencies

- `github.com/CloudyKit/jet/v6` - Template engine
- `github.com/go-sql-driver/mysql` - MySQL driver
- `github.com/sirupsen/logrus` - Logging
- `golang.org/x/text` - Text processing

## Installation Location

The tool installs to `~/bin/buildtool-model` and copies templates to `~/bin/buildtool/` for runtime access.