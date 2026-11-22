Fixtures shared by the Go code (PC) and the Kotlin tests (Android). The Go test
`internal/golden` writes the `*.json` / `*.txt` expectations from the `.md` inputs:

    go test ./internal/golden -update

The Kotlin unit tests (android/app/src/test) parse the same inputs and must produce the same
results, so a script, a brief or a sync hash means the same thing on both sides.
