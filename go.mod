module digital.vasic.helixmemory

go 1.25

require (
	digital.vasic.memory v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.6.0
	github.com/prometheus/client_golang v1.22.0
	github.com/stretchr/testify v1.11.1
	go.uber.org/zap v1.27.1
	golang.org/x/sync v0.14.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.37.1
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	modernc.org/libc v1.65.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// CONST-051(B/C) + CONST-052: vasic-digital/Memory lives at <repo_root>/submodules/memory
// per HelixCode's canonical flat lowercase_snake_case layout (sibling dir ../memory).
// Replace directive is a consumer-side build override only; adapter.go imports the canonical
// module path digital.vasic.memory/pkg/store (no source-level coupling to the parent project).
replace digital.vasic.memory => ../memory
