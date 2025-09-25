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
- `make clean` - Cleans build artifacts from `~/bin/*`

### Running the Tool
The main executable takes these arguments:
```bash
buildtool-model <target_path> [language] [package_name]
```
- `target_path`: Root directory of the target project
- `language`: "go" (default), "dart"/"flutter", or "kotlin"/"spring"
- `package_name`: Optional package name (auto-detected from go.mod if not provided)

## Architecture

### Core Components

1. **Configuration System** (`config/`)
   - `config.json`: Database connection and generation settings
   - `config.go`: Configuration structures and JSON parsing

2. **Template Engine** (`views/`)
   - Uses CloudyKit Jet templating engine
   - `views/go/`: Go templates (model.jet, rest.jet, const.jet)
   - `views/dart/`: Dart/Flutter templates (model.jet, params.jet, provider.jet, repository.jet)
   - `views/kotlin/`: Kotlin/Spring Boot templates (entity.jet, repository.jet, service.jet, controller.jet)

3. **Code Generation** (`main.go`, `go/`, `dart/`, `kotlin/`)
   - Connects to MySQL database via `information_schema`
   - Generates models, controllers, and data access code
   - Supports custom method generation via configuration
   - Multi-language support: Go, Dart/Flutter, Kotlin/Spring Boot

### Database Integration

- Uses MySQL with connection pooling
- Queries `information_schema.tables` and `information_schema.columns`
- Generates code based on database schema and naming conventions
- Table naming: `prefix_tablename` format where prefix becomes model prefix

### Generated Code Structure

**For Go:**
- Models: `./models/{tablename}.go`
- Controllers: `./controllers/rest/{tablename}.go`
- Constants: `./models/{tablename}/{tablename}.go`

**For Dart/Flutter:**
- Generates to `../gym/gym/lib/{type}/{tablename}_{type}.dart`
- Types: model, params, provider, repository

**For Kotlin/Spring Boot:**
- Entities: `../gym/gymspring/src/main/kotlin/com/gowoobro/gymspring/entity/{ModelName}.kt`
- Repositories: `../gym/gymspring/src/main/kotlin/com/gowoobro/gymspring/repository/{ModelName}Repository.kt`
- Services: `../gym/gymspring/src/main/kotlin/com/gowoobro/gymspring/service/{ModelName}Service.kt`
- Controllers: `../gym/gymspring/src/main/kotlin/com/gowoobro/gymspring/controller/{ModelName}Controller.kt`
- Creates complete REST API with JPA entities, repositories, services, and controllers

### Configuration Schema

The `config.json` supports:
- Database connection settings
- Language selection
- Custom method generation via `table` array with GPA (Generation Parameter Array) objects
- Join configurations for related tables
- Session parameter mappings

### Type Mapping

The tool automatically maps MySQL types to target language types:
- `int` → `int` (Go) / `int` (Dart) / `Int` (Kotlin)
- `bigint` → `int64` (Go) / `int` (Dart) / `Long` (Kotlin)
- `varchar`/`text` → `string` (Go) / `String` (Dart) / `String` (Kotlin)
- `datetime`/`date` → `string` (Go) / `String` (Dart) / `LocalDateTime` (Kotlin)
- `tinyint` → `bool` (Go) / `bool` (Dart) / `Int` (Kotlin)
- `double`/`float` → `Double` (Go) / `double` (Dart) / `BigDecimal/Double` (Kotlin)
- `decimal` → `int` (Go) / `int` (Dart) / `BigDecimal` (Kotlin)

## Dependencies

- `github.com/CloudyKit/jet/v6` - Template engine
- `github.com/go-sql-driver/mysql` - MySQL driver
- `github.com/sirupsen/logrus` - Logging
- `golang.org/x/text` - Text processing

## Installation Location

The tool installs to `~/bin/buildtool-model` and copies templates to `~/bin/buildtool/` for runtime access.