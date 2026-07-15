# Test evidence

This file records only commands actually executed in this repository. Expected future
commands belong in `README.md` or `docs/implementation-plan.md`, not here.

## 2026-07-14 baseline

| Command | Result |
|---|---|
| `git status --short` | Pass: clean worktree before implementation |
| `pdfinfo SE_Project.pdf` | Pass: readable, 11 pages, unencrypted |
| `pdftotext -layout SE_Project.pdf /tmp/kiticket-se-project.txt` | Pass: complete extraction, 397 lines / 2,188 words |
| `pdftotext -layout Documents/<document>.pdf ...` | Pass: all three historical PDFs readable; 17 pages total |
| `go version` | Available: `go1.26.0 linux/amd64` |
| `docker --version` / `docker compose version` | Available: Docker 29.6.1 / Compose v5.2.0 |
| `plantuml -version` | Blocked locally: executable not installed |
| `golangci-lint --version` | Blocked locally: executable not installed |
