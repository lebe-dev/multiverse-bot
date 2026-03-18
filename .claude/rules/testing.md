---
paths:
  - "**/*_test.go"
---

# Testing conventions

- Use stdlib `testing` package — no third-party test frameworks
- Mock dependencies via local interface implementations (structs in the test file)
- Test files use `_test` suffix on the package name for black-box tests
- Use `t.Errorf` / `t.Fatalf` — no assertion libraries
- Create temp files with `os.MkdirTemp` and clean up with `defer os.RemoveAll`
- Run with `-race` flag: `just test` already includes `-v -race`
