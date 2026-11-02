# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

"gomachine" is a Go code generator. It reads a MySQL schema (`information_schema`) plus a `model.json` and generates, per table:
Go (fiber REST: models, controllers, routers, enum constants), Kotlin/Spring Boot (JPA entity, repository, service, controller),
Dart/Flutter models, React / Vue 3 TypeScript clients, and (in progress) a Rust backend (axum 0.8 + sqlx 0.9) mirroring the Go one. Templates use CloudyKit Jet. README.md (Korean) is the detailed spec of
the generated behavior; keep it in sync when behavior changes.

## Key Commands

- `make install` - builds `bin/model`, installs it to `~/bin/buildtool-model` and copies `views/*` to `~/bin/buildtool/`
- `make run` - runs the generator from source (`go run .`)
- `make model` - builds `bin/model`
- `make test` - `go test -v ./...`
- `make clean` - removes local `bin/` and what `make install` put in `~/bin`
- Before finishing a change: `gofmt -l .`, `go vet ./...`, `go test ./...`

### Running the Tool

```bash
cd <project with config/model.json>
buildtool-model [ignored] [languages] [package_name]
```
- The generator reads `./config/model.json` from the **current directory** (if it has `modelJsonPath`, that directory's `model.json` is used instead). The first argument is not used.
- `languages`: overrides model.json's selection. One or comma-separated: `go`, `kotlin`/`spring`, `dart`/`flutter`, `react`, `vue`, `rust`, or `all`
- `package_name`: Go import path prefix of the target project; defaults to model.json `database`
- Templates are loaded at runtime from `$HOME/bin/buildtool/{go,kotlin,dart,react,vue,rust}` (so tests/harnesses point `HOME` at a dir whose `bin/buildtool` links to `views/`)

## Architecture

- `main.go` - loads config, lists tables, `util.ResolveTableNames` (name collisions), `util.CheckTableConfigs` / `util.FindTableConfig` (table config matching: exact name first, then `_`/case-insensitive), pre-validates `session` columns (auth=jwt + unusable session column → exit 1 before writing anything), then runs each enabled generator per table
- `config/` - `ModelConfig` / `Gpa` structs. `config.Load`/`Parse` are strict: JSON syntax/type errors are fatal (`Init` exits 1), unknown keys only produce warnings
- `util/` - shared rules every generator must use: type mapping (`GetType`), names (`ModelName`, `FileName`, `JSONKey`, `SearchSegment`, `RangeParams`, `DedupeColumns` incl. the reserved `extra` key), enum codes, `ParseFuncs` (custom methods), `ProtectedColumns`, `ResolveJoins` (join table + local column resolution, skipped joins warn), `SessionColumnError`, `KotlinTypeName` (Kotlin class names clashing with Kotlin/Spring/JPA types get `Entity`/`Enum` suffix), Rust names in `util/rust.go` (`RustIdent`, `RustField(s)`, `RustModelName`, `RustEnumType`, `RustModule`)
- `go/`, `kotlin/`, `dart/`, `react/`, `vue/`, `rust/` - one generator per language (`Process{Lang}`); Vue reuses `react.BuildTemplateData`
- `views/go/` (model, rest, const, domain_router), `views/kotlin/` (entity, repository, service, controller, enums), `views/dart/` (model), `views/react/` (contract, model, type), `views/vue/` (contract, model, type, composable)

### Generated Code Structure

- **Go** (under `goModelFilePath`): `models/{name}.go`, `models/{name}/{name}.go` (enum constants), `controllers/rest/{name}.go`, `router/routers/{name}.go`
- **Kotlin/Spring** (under `kotlinModelFilePath`, which must be the directory of `kotlinPackage`, default package `com.gowoobro.gymspring`): `entity/`, `repository/`, `service/`, `controller/`, `enums/{name}/Enums.kt`, `support/DateParams.kt` (always regenerated), `config/SessionProvider.kt` (auth=jwt, created once, never overwritten)
- **Rust** (under `rustModelFilePath` = crate root, files go in `src/`; modules use `crate::` paths): runtime support module only so far. `rustcodegen.Prepare` (once per run, before tables) renders `views/rust/support/*.jet` to `src/support/*.rs` (regenerated, written only when content changed via `util.WriteIfChanged`; `support/auth.rs` JWT→Session is user-owned, `util.WriteIfMissing`), `src/main.rs` if neither main.rs nor lib.rs exists (user-owned), and warns about missing Cargo.toml deps; `rustcodegen.Finish` (after all tables) rewrites `src/models/mod.rs`, `src/handlers/mod.rs`, `src/routes.rs`. Per-table models/handlers are not implemented yet (`ProcessRust` only warns). The support files carry `#[cfg(test)]` Rust tests
- **Dart** (under `dartModelFilePath`): `{table}.dart`
- **React / Vue** (under `reactModelFilePath` / `vueModelFilePath`): `src/models/{name}.ts`, `src/models/contract.ts`, `src/types/{name}.ts`, Vue also `src/composables/use{Model}.ts`
- Non-Go languages without an output path are skipped with a warning

### Configuration (`model.json`)

- DB connection (`server`, `database`, `user`, `password`; env `DB_HOST`/`DB_USER`/`DB_PASSWORD` override)
- `languages` map (`{"go": true, "spring": true}`, `"all": true`), falling back to legacy `language`, then `go`
- `auth` (`"jwt"` → login required everywhere except User create), `adminLevel`, `timezone` (createddate / Spring date conversion; empty = Asia/Seoul)
- `kotlinPackage`, output paths (`goModelFilePath`, `kotlinModelFilePath`, `dartModelFilePath`, `reactModelFilePath`, `vueModelFilePath`, `rustModelFilePath`), `modelJsonPath`
- `table[]`: `name`, `map` (enums), `method` (Go custom methods), `join`, `compare`, `session`, `search`, `primary`, `protect`

### Type Mapping

`util.GetType` maps MySQL `data_type` per language (`GetTypeUnsigned` also takes `column_type`'s `unsigned`, used by Rust only). Unknown types log a warning and fall back to string.

| MySQL | Go | Kotlin | Dart | TypeScript | Rust |
|---|---|---|---|---|---|
| `int`, `integer`, `smallint`, `mediumint`, `year` | `int` | `Int` | `int` | `number` | `i32` (`u32` if unsigned) |
| `bigint` | `int64` | `Long` | `int` | `number` | `i64` (`u64` if unsigned) |
| `tinyint`, `bit` | `bool` | `Boolean` | `bool` | `boolean` | `bool` |
| `float`, `double`, `real` | `float64` | `Double` | `double` | `number` | `f64` |
| `decimal`, `numeric` | `float64` | `BigDecimal` | `double` | `number` | `f64` |
| `char`, `varchar`, `*text`, `enum`, `set`, `json` | `string` | `String` | `String` | `string` | `String` |
| `datetime`, `timestamp` | `string` | `LocalDateTime` | `String` | `string` | `String` |
| `date` | `string` | `LocalDate` | `String` | `string` | `String` |
| `time` | `string` | `String` | `String` | `string` | `String` |
| `binary`, `varbinary`, `*blob` | `[]byte` | `ByteArray` | `String` | `string` | `Vec<u8>` |

Go reads `bit` columns as `(col+0)`. Columns in `table[].map` become enum types (codes = position in `data`).

### Naming

- Tables: `_tb` / `_vw` stripped (`user_role_tb` → `UserRole`, files/URLs `userrole`). `_vw` is read-only. Name collisions: `_tb` keeps the name, others are renamed (`UserView`); config must use the full table name for renamed tables.
- Columns: prefix before the first `_` is dropped; API name = `util.JSONKey` (lowerCamel). Every generator must use `JSONKey` / `SearchSegment` / `RangeParams` / `DedupeColumns`; `json_keys_test.go` and `enum_codes_test.go` check the languages agree.
- List filters / `/search`: never on `id`, `primary` columns, password, binary columns (all backends and TS agree).
- Kotlin controllers name all locals/params with a leading `_` (column properties never contain `_`) and bind query params with explicit `@RequestParam("name")`.

### HTTP contract

React/Vue/Dart must work against both Go and Spring. README.md "HTTP 계약" documents routes, response shapes and errors. On the Go side only add routes/fields; register static routes before `/:id`.

## Testing

- `go test ./...` (unit tests in `util`, `config`, `go`, and cross-language tests in the root package that render templates into a temp dir with `HOME` pointing at `views/`)
- Generated code is verified out of tree: Go output with `go build`/`go vet` in a stub project, Kotlin output with `kotlinc` against Spring/JPA/Jackson/Hibernate jars, TS with `tsc --strict`

## Dependencies

- `github.com/CloudyKit/jet/v6` - templates
- `github.com/go-sql-driver/mysql` - MySQL driver
- `github.com/sirupsen/logrus` - logging
- `gopkg.in/yaml.v3` - pubspec reading
- `golang.org/x/text` - listed in go.mod
