# Java API Surface (Scaffold)

This document describes the initial Java scaffold for `local-go-chroma`.

## Modules

- `java/core` (Java 17)
  - `tech.amikos.chroma.local.core.ChromaRuntime`
  - `tech.amikos.chroma.local.core.EmbeddedSession`
  - `tech.amikos.chroma.local.core.ChromaException`
- `java/jna` (Java 17)
  - `tech.amikos.chroma.local.jna.JnaChromaRuntime`
- `java/panama` (Java 22)
  - `tech.amikos.chroma.local.panama.PanamaChromaRuntime`

## Implemented Surface

- Runtime initialization from explicit library path.
- Shim version lookup (`chroma_version`).
- Embedded startup from YAML string (`chroma_embedded_start_from_string`).
- Embedded lifecycle close (`chroma_embedded_free`).
- Error retrieval (`chroma_get_last_error`) and string free (`chroma_string_free`).

## Not Yet Implemented

- Collection/document APIs parity with Go.
- Server mode APIs.
- Request/response model parity and advanced configuration options.

## Compatibility

- JNA backend targets Java 17+.
- Panama backend targets Java 22+ (`--enable-native-access=ALL-UNNAMED` for tests).
- Both backends expect `CHROMA_LIB_PATH` to point at one of:
  - Linux: `libchroma_shim.so`
  - macOS: `libchroma_shim.dylib`
  - Windows: `chroma_shim.dll`

## Versioning

- Local builds default to Java artifact version `0.0.0-SNAPSHOT`.
- Release builds can override version with Gradle property `releaseVersion`; a leading `v` is stripped.
- Java artifacts currently track the same repository release line as the native shim.
