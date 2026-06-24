# logzip

> Log-specialized lossless compressor based on the LogShrink algorithm

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)


`logzip` is a gzip-like CLI tool that compresses `.log` files and directories containing `.log` files. 


## Key Results

| Data size | Raw | gzip | logzip+zstd | vs gzip |
|-----------|-----|------|-------------|---------|
| 268 KB    | 100%| 10.2%| **2.3%**    | **77% better** |

## Installation

### From source
```bash
go install github.com/sathyanarrayanan/logzip/cmd/logzip@latest
```

### Pre-built binary
Download the latest binary for your platform from the [Releases page](https://github.com/sathyanarrayanan/logzip/releases).

## Quick Start

```bash
# Compress a single log file (replaces original, gzip-style)
logzip app.log

# Compress, keeping the original
logzip -k app.log

# Compress a directory of .log files into a single archive
logzip -r -k logs/

# Decompress
logzip -d app.logz

# Test archive integrity
logzip -t app.logz

# Use with pipes
cat app.log | logzip -c > app.logz
logzip -dc app.logz | grep ERROR
```

## CLI Reference

| Flag | Default | Description |
|------|---------|-------------|
| `-d`, `--decompress` | — | Decompress mode |
| `-c`, `--stdout` | — | Write to stdout (implies `-k`) |
| `-k`, `--keep` | — | Keep original file |
| `-f`, `--force` | — | Overwrite existing output |
| `-r`, `--recursive` | — | Recurse into directories |
| `-1..-9`, `--level` | `6` | Backend compression level |
| `--backend` | `zstd` | Backend: `zstd`, `gzip`, `lzma`, `none` |
| `--no-backend` | — | Skip backend pass (bare columnar container) |
| `--chunk-lines` | `100000` | Lines per processing chunk |
| `--window` | `20` | Sequence window length (analyzer) |
| `-v`, `--verbose` | — | Detailed progress output |
| `-q`, `--quiet` | — | Suppress non-error output |
| `-l` | — | List archive contents |
| `-t` | — | Test archive integrity |
| `--version` | — | Print version |

## How It Works

`logzip` implements the LogShrink algorithm in four stages:

### 1. Log Parser (Drain)
Each log line is split into a **header** (timestamp, IP, etc.) and **content**. A Drain-style prefix-tree parser extracts event **templates** (e.g., `"GET /<*> HTTP/1.0"`) and separates **variables** (the `<*>` values).

### 2. Commonality & Variability Analyzer
- **LCS-based pattern mining**: Identifies common delimiter patterns in variable columns (e.g., `"/"` in URL paths). Splits columns into sub-fields for better compressibility.
- **Weighted-entropy delta encoding**: Numeric columns (response codes, sizes) are tested with weighted-entropy; if differencing reduces entropy, they're delta-encoded.
- **Dictionary encoding**: Low-cardinality string columns (HTTP methods, status codes) are replaced with dictionary indices.

### 3. Columnar Encoding
The parsed data is stored **column-oriented** (column-major layout) with per-column optimal encoding: raw strings, dictionary IDs, delta-encoded integers, or split sub-fields. Small integers use zigzag+varint encoding.

### 4. Backend Compression
The columnar payload is optionally compressed with a general-purpose backend (default: **zstd**), which benefits from the locality created by the columnar layout.

### Decompression
The `.logz` container is **self-describing** — it embeds templates, dictionaries, patterns, header schemas, and any unparseable lines (stored verbatim to guarantee losslessness). Decompression reverses each stage exactly.

## File Format (`.logz`)

```
+===================================+
| OUTER WRAPPER (optional)          |
|  magic="ZZST" + backend-id + blob |
+===================================+
| CONTAINER HEADER                  |
|  magic="LOGZ" + version + flags   |
+-----------------------------------+
| ENTRY TABLE                       |   one per original .log file
+-----------------------------------+
| GLOBAL META                       |
|  header schema + templates + dicts|
+-----------------------------------+
| CHUNK DATA SECTION[]              |   column-encoded rows
+-----------------------------------+
| FAILED-LOG STREAMS                |   verbatim unparseable lines
+===================================+
```

## Performance Notes

- **Small files** (a few KB): overhead from the container format and template table means `logzip` may be larger than raw gzip. This is expected — the algorithm is designed for production log volumes.
- **Large files** (100KB+): the columnar layout and backend compressor dramatically outperform gzip, especially on repetitive log data with many similar lines.
- The backend default is `zstd` (level 6). Use `--backend gzip` if you cannot add the zstd dependency.

## Project Structure

```
logzip/
  cmd/logzip/main.go            # CLI entry point
  internal/
    cli/flags.go                # Flag parsing
    format/                     # Container format (read/write)
    parser/                     # Drain template parser + header schema
    analyzer/                   # LCS pattern miner, entropy, dict encoding
    sampler/                    # Clustering-based sequence sampler
    compress/                   # Compression pipeline
    decompress/                 # Decompression pipeline
    backend/                    # zstd/gzip/none backends
```

## License

MIT
