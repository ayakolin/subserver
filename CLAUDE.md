# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run development server
go run main.go

# Build binary
go build -o subserver .

# Run binary
./subserver
```

Server starts at `http://localhost:8080`.

## Architecture

Single-file Go application using Gin framework:

- **main.go** - Main server logic with three routes:
  - `GET /` - Serves index.html upload page
  - `POST /upload` - Handles file upload, validates type/size, saves to `./uploads`
  - `GET /raw/:id` - Returns file content as plain text

- **index.html** - Frontend with drag-and-drop upload UI

Files are stored in `./uploads` with generated 16-char hex IDs as filenames (e.g., `a1b2c3d4e5f6g7h8.txt`). File lookup by prefix matching on the ID.

Allowed file types: yaml, yml, json, txt, toml, xml, ini, env, properties, conf, cfg, config, rc, csv, tsv, sh, bash, zsh, makefile, dockerfile, procfile, gemfile, rakefile. Max size: 1MB.
