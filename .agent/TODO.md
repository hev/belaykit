# Belay Integration TODO

- [x] Add `EventPhase` constant and `PhaseName` field to `Event` in `stream.go`
- [x] Add JSON struct tags to belay's `trace.Node` in `../belay/trace/node.go`
- [x] Add `ReadFile` and `ReadLatest` functions in `../belay/trace/read.go`
- [x] Create `providers/belay/belay.go` — ObservabilityProvider + EventHandler
- [ ] Write tests in `providers/belay/belay_test.go` for multi-phase run
- [ ] Update belay's `main.go` to optionally read from JSON file
