# Reload Path Bugfixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 4 confirmed correctness bugs in ziphttp's reload path (`SIGHUP` reload, `--autoreload` fsnotify watcher, `ArchiveOffset` self-extract/in-memory offset detection, `initialize_file` fd leak) so that reload failures are surfaced instead of silently corrupting state or permanently disabling signal handling.

**Architecture:** No new components. Each task tightens error handling in an existing function, and two tasks extract a small already-inline block into a named method/function purely to make it unit-testable (`handleSignal`, `startAutoReload` / `autoReloadLoop`). No behavior changes beyond the bug fixes themselves.

**Tech Stack:** Go 1.25, stdlib `testing` (no testify in this repo), `fsnotify/fsnotify`, `go-task` (`Taskfile.yml`), `golangci-lint` (errcheck disabled, staticcheck enabled), `gofmt`.

**Spec:** No separate spec document — this plan is driven directly by a code review conducted 2026-08-25 against `webserver.go` (SIGHUP handling, `--autoreload`, `initialize_file`) and `util.go` (`ArchiveOffset`). Each task below restates the specific finding it fixes, including exact line numbers as they were at review time.

## Global Constraints

- Go 1.25.0, module `github.com/wtnb75/ziphttp` — no new dependencies.
- No testify or other assertion library — use plain `testing`, matching existing style (`t.Parallel()` on every test **except** ones that mutate the package-level `globalOption` var, which must not run parallel and must save/restore it via `defer`, matching `TestReload`/`TestReloadError`).
- Before each commit: `task fmt` (gofmt + go vet), `task lint` (golangci-lint), `task test` (go test -v ./... with coverage). All three must pass.
- `errcheck` is disabled in `.golangci.yml` — do not rely on lint to catch unchecked errors; check them deliberately per the fixes below.
- Comments and log messages in English; commit messages/PR description in English is fine, conversational recap to the user stays Japanese per user's global preference (not a code concern, just a reminder for whoever executes this).
- Never use `t.Fatal` where the existing file's sibling tests use `t.Error` + manual `return`, unless the test can't safely continue (e.g. tempfile creation) — match each file's local convention (`util_test.go` prefers `t.Error`+`return`/`panic`; some `webserver_test.go` helpers use `t.Fatal` is acceptable for setup-only failures since no existing test in that file relies on continuing past a setup error — check the file if unsure).

---

## Grouping and ordering rationale (read before executing)

Six issues were found in review; four are fixed here as one coherent plan, one is folded into an existing task because it lives in the same code block, and one newly-discovered issue is deliberately left **out** of this plan pending user confirmation (see "Not in scope" at the end).

- **Task 1** (`util.go` `ArchiveOffset`) bundles the EOCD-not-found bug and the invalid-signature bug into one task because they're two branches of the *same function*, same root theme (log-but-don't-return), and touching the function once for both avoids two separate diffs on overlapping lines. This is the **root cause** of the in-memory/self-extract corruption the user remembers, so it goes first.
- **Task 2** (SIGHUP goroutine) is kept **separate** from Task 1 even though they're both "reload" — it's a different file, different function, and a different failure mode (goroutine death vs. bad offset). It's sequenced **right after** Task 1 on purpose: once `ArchiveOffset` correctly returns errors instead of swallowing them, `Reload()` will fail *for real* far more often in self-extract/in-memory setups, which makes "one failed reload permanently kills SIGHUP/SIGINT/SIGTERM handling" a much more likely outage. Fixing Task 1 first and Task 2 second closes that window; doing them in the opposite order would briefly make things worse (errors surface but still kill the signal handler).
- **Task 3** bundles the `fsnotify.NewWatcher()` nil-pointer risk and the Remove/Rename (atomic replace) handling into **one task** because they're the same ~35-line code block (`webserver.go:746-781`) being extracted into a testable helper anyway — splitting them would mean re-touching the same lines twice for no isolation benefit. This is sequenced third: it's independent of Tasks 1-2 in terms of correctness, but it's the deepest refactor (extracts two new functions), so doing it after the smaller, higher-confidence fixes reduces risk if something needs to be reverted.
- **Task 4** (fd leak in `initialize_file`) is fully independent of the other three — different file section, different failure mode (resource leak, not a correctness/availability bug), lowest severity. It's last because nothing else depends on it and it's the smallest change.

Each task is its own commit and can be reviewed/merged independently — none of them share code, so there's no reason to land them as a single squashed change. If time is short, Task 1 + Task 2 together are the highest-value pair (they directly explain the user-reported symptom); Tasks 3 and 4 can be deferred to a follow-up without leaving Task 1/2 half-done.

---

## Task 1: Fix `ArchiveOffset` silent failure on EOCD-not-found and invalid central-directory signature

**Files:**
- Modify: `util.go:113-154` (`ArchiveOffset`)
- Test: `util_test.go` (add after `TestArchiveOffset2`, i.e. after line 141)

**Interfaces:**
- Consumes: nothing new — `ArchiveOffset(archivefile string) (int64, error)` signature is unchanged.
- Produces: `ArchiveOffset` now returns a non-nil `error` on both failure branches below. Its one production caller, `(h *ZipHandler) initialize` at `webserver.go:605-609`, already checks `err != nil` and returns — no caller-side change needed.

**Finding being fixed:**
- `util.go:133-137` — when the EOCD signature (`PK\x05\x06`) isn't found in the last 512 bytes, `idx` stays `-1`, an error is logged, but the function falls through and computes `cdsize` from `tail[11:15]` (garbage) instead of returning an error.
- `util.go:149-151` — when the computed central-directory header doesn't start with `PK\x01\x02`, an error is logged and `return 0, err` executes, but `err` is `nil` at that point (last assigned by a successful `fp.Read` at line 144), so the caller sees `(0, nil)` — a silent success with a bogus offset.

- [ ] **Step 1: Write the failing tests**

Add to `util_test.go` (after `TestArchiveOffset2`, before `TestArchiveOffsetOld`):

```go
func TestArchiveOffsetEOCDNotFound(t *testing.T) {
	t.Parallel()
	tmpf, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Fatal("tempfile", err)
	}
	defer os.Remove(tmpf.Name())
	// 600 zero bytes: long enough for the -512-from-end seek to succeed,
	// but contains no EOCD signature anywhere.
	if _, err := tmpf.Write(make([]byte, 600)); err != nil {
		t.Fatal("write", err)
	}
	tmpf.Close()

	if _, err := ArchiveOffset(tmpf.Name()); err == nil {
		t.Error("expected error when EOCD signature is not found")
	}
}

func TestArchiveOffsetInvalidCentralDirectorySignature(t *testing.T) {
	t.Parallel()
	tmpf, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Fatal("tempfile", err)
	}
	defer os.Remove(tmpf.Name())

	// 500 zero bytes of padding, then a hand-built 22-byte EOCD record
	// whose cdsize (50) points back into the zero padding -- so the
	// "central directory head" ArchiveOffset seeks to is all zero bytes,
	// which can never start with the PK\x01\x02 signature.
	padding := make([]byte, 500)
	if _, err := tmpf.Write(padding); err != nil {
		t.Fatal("write padding", err)
	}
	eocd := make([]byte, 22)
	copy(eocd[0:4], []byte{0x50, 0x4b, 0x05, 0x06})
	binary.LittleEndian.PutUint32(eocd[12:16], 50) // cdsize
	if _, err := tmpf.Write(eocd); err != nil {
		t.Fatal("write eocd", err)
	}
	tmpf.Close()

	if _, err := ArchiveOffset(tmpf.Name()); err == nil {
		t.Error("expected error for invalid central directory signature")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestArchiveOffsetEOCDNotFound|TestArchiveOffsetInvalidCentralDirectorySignature' -v`
Expected: both `FAIL` — the first because `ArchiveOffset` returns `(garbage, nil)` instead of an error, the second because it returns `(0, nil)`.

- [ ] **Step 3: Fix `ArchiveOffset`**

In `util.go`, replace lines 133-137:

```go
	idx := bytes.LastIndex(tail[0:sz], []byte{0x50, 0x4b, 0x05, 0x06})
	if idx == -1 {
		slog.Error("end of central directory not found", "name", archivefile, "bytes", tail)
		return 0, fmt.Errorf("end of central directory not found: %s", archivefile)
	}
	cdsize := binary.LittleEndian.Uint32(tail[idx+0xc : idx+0xc+4])
```

And replace lines 149-152:

```go
	if !bytes.HasPrefix(cdhead, []byte{0x50, 0x4b, 0x1, 0x2}) {
		slog.Error("invalid signature", "signature", cdhead[0:4])
		return 0, fmt.Errorf("invalid central directory signature: %x", cdhead[0:4])
	}
```

(`fmt` is already imported in `util.go`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestArchiveOffset' -v`
Expected: `PASS` for all five `TestArchiveOffset*` tests (the two new ones plus the three existing ones, which must still pass unchanged).

- [ ] **Step 5: Full verification and commit**

```bash
task fmt
task lint
task test
git add util.go util_test.go
git commit -m "fix: ArchiveOffset returns error instead of silently swallowing EOCD/signature failures"
```

---

## Task 2: Stop SIGHUP reload failure from killing the signal-handling goroutine

**Files:**
- Modify: `webserver.go:726-744` (inline `go func(){...}()` inside `Execute`)
- Test: `webserver_test.go` (add near `TestReloadError`, i.e. after line 667)

**Interfaces:**
- Consumes: `cmd.Reload() error` (existing, `webserver.go:803-811`), `cmd.Shutdown() error` (existing, `webserver.go:798-801`).
- Produces: new method `func (cmd *WebServer) handleSignal(sig os.Signal) (exit bool)` on `*WebServer`. `exit == true` means the caller should stop reading from the signal channel and return (mirrors today's `SIGINT`/`SIGTERM` behavior); `exit == false` means keep looping (mirrors today's successful-`SIGHUP` behavior, but now also covers the *failed*-`SIGHUP` case).

**Finding being fixed:** `webserver.go:731-736` — on `SIGHUP`, if `cmd.Reload()` returns an error, the `case` block logs it and then `return`s, which exits the `for` loop and ends the entire goroutine. Since the same goroutine also handles `SIGINT`/`SIGTERM` graceful shutdown, one failed reload permanently disables both further reloads and graceful shutdown (only `SIGKILL` works afterward).

- [ ] **Step 1: Write the failing test**

Add to `webserver_test.go` (after `TestReloadError`):

```go
func TestHandleSignalReloadFailureDoesNotExit(t *testing.T) {
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename("/not/found/archive-for-sighup.zip")

	cmd := WebServer{handler: ZipHandler{methodmap: make(map[string]map[uint16]int)}}

	exit := cmd.handleSignal(syscall.SIGHUP)
	if exit {
		t.Error("expected handleSignal(SIGHUP) to return false (keep listening) even when Reload fails")
	}
}

func TestHandleSignalTermExits(t *testing.T) {
	t.Parallel()
	cmd := WebServer{}

	exit := cmd.handleSignal(syscall.SIGTERM)
	if !exit {
		t.Error("expected handleSignal(SIGTERM) to return true (stop listening)")
	}
}
```

(`syscall` needs to be added to `webserver_test.go`'s import block.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestHandleSignal' -v`
Expected: compile failure — `handleSignal` doesn't exist yet. That's the expected "fail" for a not-yet-extracted method; confirm it's a missing-symbol compile error, not something else.

- [ ] **Step 3: Extract `handleSignal` and fix the early `return`**

In `webserver.go`, replace lines 726-744:

```go
	go func() {
		for {
			sig := <-sigs
			slog.Info("caught signal", "signal", sig)
			if cmd.handleSignal(sig) {
				return
			}
		}
	}()
```

And add the new method near `Reload`/`Shutdown` (e.g. directly above `func (cmd *WebServer) Reload() error` at line 803):

```go
// handleSignal processes one OS signal and reports whether the caller
// should stop listening for further signals.
func (cmd *WebServer) handleSignal(sig os.Signal) (exit bool) {
	switch sig {
	case syscall.SIGHUP:
		if err := cmd.Reload(); err != nil {
			slog.Error("reload failed", "error", err)
		}
		return false
	case syscall.SIGINT, syscall.SIGTERM:
		if err := cmd.Shutdown(); err != nil {
			slog.Error("terminate failed", "error", err)
		}
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestHandleSignal|TestReload' -v`
Expected: `PASS` for both new tests and the pre-existing `TestReload`/`TestReloadError`.

- [ ] **Step 5: Full verification and commit**

```bash
task fmt
task lint
task test
git add webserver.go webserver_test.go
git commit -m "fix: SIGHUP reload failure no longer kills the signal-handling goroutine"
```

---

## Task 3: Fix `--autoreload` watcher setup error handling and atomic-replace (rename) support

**Files:**
- Modify: `webserver.go:746-781` (inline `AutoReload` block inside `Execute`)
- Test: `webserver_test.go` (add near the end of the file, after `createSimpleZip`, i.e. after line 796)

**Interfaces:**
- Consumes: `archiveFilename() string` (existing, `main.go:18-28`), `cmd.Reload() error` (existing).
- Produces:
  - `func (cmd *WebServer) startAutoReload() (*fsnotify.Watcher, error)` — replaces the inline `fsnotify.NewWatcher()` + goroutine-spawn + `wt.Add()` block. Returns `(nil, err)` on any setup failure (never a non-nil error alongside a watcher the caller must still close, and never a nil watcher alongside a nil error).
  - `func autoReloadLoop(wt *fsnotify.Watcher, cmd *WebServer, watchPath string)` — the event loop, now handling `fsnotify.Remove`/`fsnotify.Rename` by re-arming the watch via `wt.Add(watchPath)` before reloading, in addition to the existing `fsnotify.Write` handling.

**Findings being fixed:**
- `webserver.go:747-751` — if `fsnotify.NewWatcher()` fails, the code logs and continues instead of returning; `wt` is `nil`, and the subsequent `defer wt.Close()`, the goroutine's `wt.Events`/`wt.Errors` reads, and `wt.Add()` all dereference a nil `*fsnotify.Watcher`, panicking the process.
- `webserver.go:753-773` — the loop only reacts to `fsnotify.Write`. fsnotify watches by inode, so an atomic replace (`write to tmp file` + `os.Rename` over the watched path — a standard safe-deploy technique) invalidates the watch permanently; no further events are ever delivered for that path, silently disabling `--autoreload` until process restart. This matches the user's own account: replacing a file in place (same inode) works, but a self-extract binary deployed via rename does not.

- [ ] **Step 1: Write the failing tests**

Add to `webserver_test.go`:

```go
func TestStartAutoReloadAddError(t *testing.T) {
	t.Parallel()
	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename("/not/found/archive-for-watch.zip")

	cmd := WebServer{handler: ZipHandler{methodmap: make(map[string]map[uint16]int)}}
	wt, err := cmd.startAutoReload()
	if err == nil {
		t.Error("expected error when the watch target does not exist")
	}
	if wt != nil {
		t.Error("expected nil watcher alongside a non-nil error")
		wt.Close()
	}
}

func waitForMethod(t *testing.T, h *ZipHandler, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h.rwlock.RLock()
		_, ok := h.methodmap[name]
		h.rwlock.RUnlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %q to appear via autoreload", name)
}

func TestAutoReloadLoopSurvivesAtomicRename(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "archive.zip")
	if err := createSimpleZip(zipPath, "a.txt", []byte("v1")); err != nil {
		t.Fatal("create initial zip", err)
	}

	oldArchive := globalOption.Archive
	oldSelf := globalOption.Self
	defer func() {
		globalOption.Archive = oldArchive
		globalOption.Self = oldSelf
	}()
	globalOption.Self = false
	globalOption.Archive = flags.Filename(zipPath)

	cmd := WebServer{handler: ZipHandler{methodmap: make(map[string]map[uint16]int)}}
	if err := cmd.handler.initialize([]string{zipPath}, false); err != nil {
		t.Fatal("initial initialize", err)
	}
	defer cmd.handler.Close()

	wt, err := cmd.startAutoReload()
	if err != nil {
		t.Fatal("startAutoReload", err)
	}
	defer wt.Close()

	// Atomic replace #1: write to a tmp file, then rename over zipPath.
	tmp1 := zipPath + ".tmp1"
	if err := createSimpleZip(tmp1, "b.txt", []byte("v2")); err != nil {
		t.Fatal("create tmp1", err)
	}
	if err := os.Rename(tmp1, zipPath); err != nil {
		t.Fatal("rename1", err)
	}
	waitForMethod(t, &cmd.handler, "b.txt", 2*time.Second)

	// Atomic replace #2: proves the watch was re-armed after replace #1,
	// not just that the first rename happened to still be observed.
	tmp2 := zipPath + ".tmp2"
	if err := createSimpleZip(tmp2, "c.txt", []byte("v3")); err != nil {
		t.Fatal("create tmp2", err)
	}
	if err := os.Rename(tmp2, zipPath); err != nil {
		t.Fatal("rename2", err)
	}
	waitForMethod(t, &cmd.handler, "c.txt", 2*time.Second)
}
```

(`time` and `filepath` are already imported in `webserver_test.go`; no new imports needed for this file.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestStartAutoReload|TestAutoReloadLoop' -v`
Expected: compile failure (`startAutoReload` doesn't exist yet) for both new tests.

- [ ] **Step 3: Extract `startAutoReload`/`autoReloadLoop` and fix both bugs**

In `webserver.go`, replace lines 746-781:

```go
	if cmd.AutoReload {
		wt, err := cmd.startAutoReload()
		if err != nil {
			return err
		}
		defer wt.Close()
	}
```

Add the two new functions (e.g. directly above `func (cmd *WebServer) Shutdown() error` at line 798):

```go
// startAutoReload sets up the fsnotify watcher and its event loop for
// --autoreload. On any setup failure it returns (nil, err); it never
// returns a watcher the caller must clean up alongside a non-nil error.
func (cmd *WebServer) startAutoReload() (*fsnotify.Watcher, error) {
	wt, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("watcher", "error", err)
		return nil, err
	}
	path := archiveFilename()
	go autoReloadLoop(wt, cmd, path)
	if err := wt.Add(path); err != nil {
		slog.Error("watcher add", "error", err)
		wt.Close()
		return nil, err
	}
	return wt, nil
}

// autoReloadLoop reacts to filesystem events on the watched archive.
// fsnotify watches by inode, so a Write (in-place modification) is
// handled directly, while Remove/Rename (e.g. an atomic replace via
// write-to-tmp + os.Rename) requires re-arming the watch on the new
// inode at the same path before reloading.
func autoReloadLoop(wt *fsnotify.Watcher, cmd *WebServer, watchPath string) {
	for {
		select {
		case event, ok := <-wt.Events:
			if !ok {
				slog.Error("watcher events channel closed")
				return
			}
			slog.Info("got watcher event", "event", event, "op", event.Op.String())
			switch {
			case event.Has(fsnotify.Write):
				slog.Info("modified", "name", event.Name)
				if err := cmd.Reload(); err != nil {
					slog.Error("reload error", "error", err)
				}
			case event.Has(fsnotify.Remove), event.Has(fsnotify.Rename):
				slog.Info("watched file replaced, re-arming watch", "name", event.Name)
				if err := wt.Add(watchPath); err != nil {
					slog.Error("re-add watcher", "error", err)
				}
				if err := cmd.Reload(); err != nil {
					slog.Error("reload error", "error", err)
				}
			}
		case err, ok := <-wt.Errors:
			if !ok {
				slog.Error("watcher errors channel closed")
				return
			}
			slog.Info("got watcher error", "error", err)
		}
	}
}
```

Remove the now-unused `fsnotify` import only if it becomes unused in `webserver.go` — it will still be used by the `*fsnotify.Watcher` return type, so no import changes are needed there.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestStartAutoReload|TestAutoReloadLoop' -v`
Expected: `PASS` for both. The rename test may take up to ~4s (two 2s timeouts as an upper bound); that's expected, not a hang.

- [ ] **Step 5: Full verification and commit**

```bash
task fmt
task lint
task test
git add webserver.go webserver_test.go
git commit -m "fix: --autoreload no longer panics on watcher setup failure and survives atomic file replacement"
```

---

## Task 4: Fix file descriptor leak in `initialize_file` on partial failure

**Files:**
- Modify: `webserver.go:577-588` (`(h *ZipHandler) initialize_file`)
- Test: `webserver_test.go` (add after `createSimpleZip`, i.e. after Task 3's additions — order in the file doesn't matter, append at end)

**Interfaces:**
- Consumes: `NewZipFileFile(name string) (*ZipFileFile, error)` (existing, `webserver.go:86-93`), `ZipFile.Close() error` (existing interface method).
- Produces: no signature change — `initialize_file` still returns `error`; only its failure-path behavior changes (closes already-opened files before returning).

**Finding being fixed:** `webserver.go:577-588` — when opening the Nth file (via `--add`, multiple zip files) fails, the loop returns immediately without closing the `ZipFile`s already opened for files `1..N-1`. Each of those wraps an open `os.File` (via `zip.OpenReader`), so every failed multi-file reload leaks one file descriptor per already-opened archive.

- [ ] **Step 1: Write the failing test**

Add to `webserver_test.go`:

```go
func openFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skip("cannot inspect /dev/fd on this platform:", err)
	}
	return len(entries)
}

func TestInitializeFilePartialFailureClosesOpenedFiles(t *testing.T) {
	dir := t.TempDir()
	zip1 := filepath.Join(dir, "a.zip")
	if err := createSimpleZip(zip1, "a.txt", []byte("hello")); err != nil {
		t.Fatal("create zip1", err)
	}
	missing := filepath.Join(dir, "does-not-exist.zip")

	h := ZipHandler{methodmap: make(map[string]map[uint16]int)}

	before := openFDCount(t)
	if err := h.initialize_file([]string{zip1, missing}); err == nil {
		t.Error("expected error for missing second file")
	}
	after := openFDCount(t)
	if after > before {
		t.Errorf("leaked file descriptors: before=%d after=%d", before, after)
	}
}
```

(`/dev/fd` is available on both Linux and macOS; on any platform where it isn't, the test skips rather than failing.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestInitializeFilePartialFailureClosesOpenedFiles' -v`
Expected: `FAIL` — `after > before` (one fd leaked for `zip1`).

- [ ] **Step 3: Fix `initialize_file`**

In `webserver.go`, replace lines 577-588:

```go
func (h *ZipHandler) initialize_file(input []string) error {
	zipfiles := make([]ZipFile, 0)
	for _, v := range input {
		zipfile, err := NewZipFileFile(v)
		if err != nil {
			for _, opened := range zipfiles {
				if cerr := opened.Close(); cerr != nil {
					slog.Error("close zipfile after partial failure", "error", cerr)
				}
			}
			return err
		}
		zipfiles = append(zipfiles, zipfile)
	}
	h.init2(zipfiles)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestInitializeFilePartialFailureClosesOpenedFiles' -v`
Expected: `PASS`.

- [ ] **Step 5: Full verification and commit**

```bash
task fmt
task lint
task test
git add webserver.go webserver_test.go
git commit -m "fix: close already-opened zip files when initialize_file fails partway through"
```

---

## Not in scope (flag to user before doing anything)

While gathering context for Task 3, a **7th, previously unreported** issue turned up in `(h *ZipHandler) init2`, `webserver.go:541-546`:

```go
if fi == nil {
	slog.Error("not found", "name", fname, "idx", idx)
}
if crc32 == 0 {
	crc32 = fi.CRC32   // nil pointer dereference if fi == nil
```

This is a potential nil-pointer panic during the CRC cross-check that runs on every `init2` call (i.e. every successful reload), not just the self-extract/in-memory path. It wasn't part of the original 6 reviewed findings and hasn't been independently verified against a concrete repro (need to confirm what input actually drives `fi == nil` — likely a `methodmap` entry whose index falls outside every input's cumulative `Files()` range). Recommend treating this as a separate Task 5 / separate plan once confirmed, rather than folding it into this one silently.
