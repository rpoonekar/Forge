# Forge

A distributed CI/build execution engine written in Go.

## Stage 1 — Single Worker, In-Memory

### Run

```bash
go run .
```

### API

**Submit a task:**
```bash
curl -X POST -d '{"command":"echo hello world"}' http://localhost:8080/submit
```

**Check task status:**
```bash
curl http://localhost:8080/task/{id}
```

**List all tasks:**
```bash
curl http://localhost:8080/tasks
```

### What to test

```bash
# Submit a few tasks
curl -X POST -d '{"command":"echo hello"}' http://localhost:8080/submit
curl -X POST -d '{"command":"sleep 3 && echo done"}' http://localhost:8080/submit
curl -X POST -d '{"command":"ls -la"}' http://localhost:8080/submit
curl -X POST -d '{"command":"exit 1"}' http://localhost:8080/submit  # this should FAIL

# Check all tasks
curl http://localhost:8080/tasks | jq .
```

