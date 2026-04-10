---
name: ziphttp
description: 'Use ziphttp CLI commands to create zip archives, list entries, serve archives over HTTP, extract gzip streams, rewrite links, and sort archives. Use when: operating zip files from the command line, validating archive contents, or running zip-backed web hosting.'
argument-hint: 'Describe your goal (create/list/serve/extract/sort/rewrite), input files, and expected output'
---

# ziphttp Command Operations

## What This Skill Does

This skill helps you run `ziphttp` commands effectively for day-to-day archive operations:
- Create or update archives
- Inspect archive entries
- Serve archive content as a website
- Extract pre-compressed data streams
- Rewrite links for static site compatibility
- Reorder archive entries

## When to Use

- You need to package files into a zip archive with compression controls
- You want to inspect archive content quickly from CLI
- You want to host static content directly from a zip file
- You need to transform or optimize an existing archive

## Required Inputs

Provide these before running commands:
1. Task goal (`create`, `list`, `serve`, `extract`, `sort`, or `rewrite`)
2. Archive path (`-f` / `--archive`) or `--self` behavior
3. Source files or directories (when creating archives)
4. Target options (port, compression method, filters, output style)

## Command Map

- `ziphttp zip`: create or update an archive from files/dirs/other zips
- `ziphttp ziplist`: list entries in the target archive
- `ziphttp webserver`: serve archive files over HTTP
- `ziphttp zip2gzip`: emit gzip stream from zip entries without full recompression
- `ziphttp zipsort`: reorder entries in an archive
- `ziphttp testlink`: rewrite HTML links relative to a base URL
- `ziphttp version`: print version and exit

## Standard Workflow

1. Confirm global options and archive target.
- Use `-f <archive.zip>` for explicit archive path.
- Use `--self` only when operating on executable-embedded archives.
- Add `-v` for debug logs or `-q` for quieter output.

2. Validate command help before execution.
- Run `ziphttp --help` for global options.
- Run `ziphttp <subcommand> --help` for command-specific flags.

3. Execute the operation.
- For archive creation: run `ziphttp -f output.zip zip <inputs...>`.
- For listing: run `ziphttp -f output.zip ziplist`.
- For serving: run `ziphttp -f output.zip webserver --help` first, then start server with desired flags.

4. Verify the result.
- Check exit status and error messages.
- Re-run `ziplist` to confirm expected entries after write/sort operations.
- For web serving, validate key routes with `curl` or browser checks.

5. Troubleshoot with focused retries.
- If archive open fails, verify path and permissions.
- If output is unexpected, rerun with `-v` and reduced option set.
- If behavior differs by subcommand, isolate with minimal input files.

## Decision Rules

- Use `zip` when you are building or merging archive content.
- Use `zipsort` when only entry order needs to change.
- Use `zip2gzip` when downstream expects gzip payload from existing zip data.
- Use `testlink` when static HTML links must be made relative to a base URL.
- Use `webserver` for runtime content delivery directly from archive files.

## Completion Checklist

- Chosen subcommand matches the requested task
- Command includes correct archive target (`-f` or `--self`)
- Relevant help output was checked before execution
- Expected output is validated (entries, HTTP responses, or transformed files)
- Verbose logs reviewed when diagnosing failures

## Example Prompts

- "Use ziphttp to package `public/` into `site.zip` with brotli compression and show verification steps."
- "List all files in `assets.zip` and explain the meaning of each output column."
- "Run ziphttp webserver for `hugo.zip` and give me a quick endpoint check plan."
