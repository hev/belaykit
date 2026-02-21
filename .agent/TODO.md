# Code Review

## Issues Found

- [ ] **Path traversal in `writeTrace`** (`providers/belay/belay.go:263`): `EndTrace` accepts a caller-provided `traceID` and uses it directly in `filepath.Join(p.dir, traceID+".json")`. Sanitize the traceID (e.g. `filepath.Base`) to prevent directory traversal.
- [ ] **No traceID validation in `EndTrace`** (`providers/belay/belay.go:118`): `EndTrace` doesn't verify the provided `traceID` matches `p.traceID`. A mismatched ID writes the current trace under the wrong filename. Add a guard or use the internal `p.traceID`.
- [ ] **Silent error swallowing in `writeTrace`** (`providers/belay/belay.go:253-264`): `os.MkdirAll` and `os.WriteFile` errors are silently discarded with no logging. Add `log.Printf` or accept a logger so callers can diagnose write failures.
- [ ] **Unchecked `json.Unmarshal` in tests** (`providers/belay/belay_test.go:161,202,276,302,365`): Several tests ignore the `json.Unmarshal` error, which masks deserialization failures. Add `if err != nil { t.Fatal(err) }` checks.
- [ ] **`StartTrace` silently overwrites active trace** (`providers/belay/belay.go:99`): Calling `StartTrace` while a trace is in progress discards the previous trace without finalizing. Document the single-trace limitation or finalize the previous trace automatically.
